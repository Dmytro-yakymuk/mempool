package console

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
	"github.com/zeebo/errs"
	"golang.org/x/sync/errgroup"

	"mempool/console/common"
	"mempool/internal/logger"
	"mempool/pkg/rate"
)

var (
	// Error is an error class that indicates internal http server error.
	Error = errs.Class("console web server error")
)

// Config contains configuration for console web server.
type Config struct {
	Address string
	Cors    struct {
		AllowedForAllOrigins bool     `env:"ALLOWED_FOR_ALL_ORIGINS"`
		AllowedOrigins       []string `env:"ALLOWED_ORIGINS" envSeparator:" "`
	} `envPrefix:"CORS_"`
	RateLimiter rate.Config `envPrefix:"RATE_LIMITER_"`
}

// Server represents console web server.
//
// architecture: Endpoint.
type Server struct {
	log    logger.Logger
	config Config

	listener    net.Listener
	server      http.Server
	rateLimiter *rate.Limiter
}

// NewServer is a constructor for console web server.
func NewServer(
	config Config,
	log logger.Logger,
	listener net.Listener,
	rateLimiter *rate.Limiter,
) *Server {
	server := &Server{
		log:         log,
		config:      config,
		listener:    listener,
		rateLimiter: rateLimiter,
	}

	router := mux.NewRouter()
	router.Use(server.rateLimit)

	apiRouter := router.PathPrefix("/api/v1").Subrouter()

	apiRouter.HandleFunc("/address/{address}/utxo", server.getUTXO).Methods(http.MethodGet)

	if !config.Cors.AllowedForAllOrigins {
		c := cors.New(cors.Options{
			AllowedOrigins:   config.Cors.AllowedOrigins,
			AllowCredentials: true,
			AllowedMethods:   []string{http.MethodGet, http.MethodDelete, http.MethodPost, http.MethodPatch, http.MethodOptions, http.MethodPut},
		})

		server.server = http.Server{
			Handler: c.Handler(router),
		}

		return server
	} else {
		router.Use(func(handler http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				server.appHandler(w, r)
				handler.ServeHTTP(w, r)
			})
		})

		server.server = http.Server{
			Handler: cors.AllowAll().Handler(router),
		}

		return server
	}
}

type UTXO struct {
	Txid         string  `json:"txid"`
	Vout         int     `json:"vout"`
	ScriptPubKey string  `json:"scriptPubKey"`
	Amount       float64 `json:"amount"`
	Height       int     `json:"height"`
}

type ScanResult struct {
	Success       bool    `json:"success"`
	SearchedItems int     `json:"searched_items"`
	Unspents      []UTXO  `json:"unspents"`
	TotalAmount   float64 `json:"total_amount"`
}

// getUTXO is an endpoint for getting utxo by address.
func (s *Server) getUTXO(w http.ResponseWriter, r *http.Request) {

	params := mux.Vars(r)
	address, ok := params["address"]
	if !ok {
		common.NewErrResponse(http.StatusBadRequest, errs.New("address is required")).Serve(s.log, Error, w)
		return
	}

	cmd := exec.Command("bitcoin-cli", "scantxoutset", "start", fmt.Sprintf("[{\"desc\":\"addr(%s)\"}]", address))

	output, err := cmd.Output()
	if err != nil {
		common.NewErrResponse(http.StatusInternalServerError, Error.Wrap(err)).Serve(s.log, Error, w)
		return
	}

	var result ScanResult
	if err := json.Unmarshal(output, &result); err != nil {
		common.NewErrResponse(http.StatusInternalServerError, Error.Wrap(err)).Serve(s.log, Error, w)
		return
	}

	if err := json.NewEncoder(w).Encode(result); err != nil {
		common.NewErrResponse(http.StatusInternalServerError, Error.Wrap(err)).Serve(s.log, Error, w)
		return
	}
}

// Run starts the server that host webapp and api endpoint.
func (server *Server) Run(ctx context.Context) (err error) {
	var group errgroup.Group

	ctx, cancel := context.WithCancel(ctx)

	group.Go(func() error {
		<-ctx.Done()
		return Error.Wrap(server.server.Shutdown(ctx))
	})

	group.Go(func() error {
		server.rateLimiter.Run(ctx)
		return nil
	})

	group.Go(func() error {
		defer cancel()

		err = server.server.Serve(server.listener)
		isCancelled := errs.IsFunc(err, func(err error) bool { return errors.Is(err, context.Canceled) })
		if isCancelled || errors.Is(err, http.ErrServerClosed) {
			err = nil
		}

		return Error.Wrap(err)
	})

	return Error.Wrap(group.Wait())
}

// Close closes server and underlying listener.
func (server *Server) Close() error {
	return Error.Wrap(server.server.Close())
}

// appHandler is web app http handler function.
func (server *Server) appHandler(w http.ResponseWriter, _ *http.Request) {
	header := w.Header()
	allowedHeaders := "Accept, Origin, Content-Type, X-Requested-With,Content-Length, Accept-Encoding, Authorization,X-CSRF-Token, Access-Control-Allow-Headers, Access-Control-Request-Method, Access-Control-Request-Headers"
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Referrer-Policy", "same-origin")
	header.Set("Access-Control-Allow-Origin", "*")
	header.Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
	header.Set("Access-Control-Allow-Headers", allowedHeaders)
	header.Set("Access-Control-Expose-Headers", "Authorization")
}

// jsonResponse sets a response' "Content-Type" value as "application/json"
func (server *Server) jsonResponse(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		handler.ServeHTTP(w, r.Clone(r.Context()))
	})
}

// rateLimit is a middleware that prevents from multiple requests from single ip address.
func (server *Server) rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, err := server.getIP(r)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		isAllowed := server.rateLimiter.IsAllowed(ip)
		if !isAllowed {
			server.log.Debug("rate limit exceeded, ip:" + ip)
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (server *Server) getIP(r *http.Request) (ip string, err error) {
	ips := r.Header.Get("X-Forwarded-For")
	splitIps := strings.Split(ips, ",")

	if len(splitIps) > 0 {
		// get last IP in list since ELB prepends other user defined IPs, meaning the last one is the actual client IP.
		netIP := net.ParseIP(splitIps[len(splitIps)-1])
		if netIP != nil {
			return netIP.String(), nil
		}
	}

	ip, _, err = net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "", err
	}

	netIP := net.ParseIP(ip)
	if netIP != nil {
		ip := netIP.String()
		if ip == "::1" {
			return "127.0.0.1", nil
		}
		return ip, nil
	}

	return "", errors.New("IP not found")
}
