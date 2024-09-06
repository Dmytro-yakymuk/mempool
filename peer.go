package mempool

import (
	"context"
	"net"

	"github.com/zeebo/errs"
	"golang.org/x/sync/errgroup"

	"mempool/console"
	"mempool/internal/logger"
	"mempool/pkg/rate"
)

// Config is the global configuration for mempool launchpad.
type Config struct {
	Console       console.Config
	BTCRPCAddress string
}

// Peer is the representation of a server.
type Peer struct {
	Config Config
	Log    logger.Logger

	// Console web server with web UI.
	Console struct {
		Listener net.Listener
		Endpoint *console.Server
	}
}

// New is a constructor for peer.
func New(ctx context.Context, logger logger.Logger, config Config) (peer *Peer, err error) {
	peer = &Peer{
		Log:    logger,
		Config: config,
	}

	// console setup
	{
		peer.Console.Listener, err = net.Listen("tcp", config.Console.Address)
		if err != nil {
			return &Peer{}, err
		}

		rateLimiter := rate.NewLimiter(config.Console.RateLimiter)

		peer.Console.Endpoint = console.NewServer(
			config.Console,
			peer.Log,
			peer.Console.Listener,
			rateLimiter,
		)
	}

	return peer, nil
}

// Run runs console until it's either closed or it errors.
func (peer *Peer) Run(ctx context.Context) error {
	peer.Log.Debug("mempool running")

	group, ctx := errgroup.WithContext(ctx)

	group.Go(func() error {
		return peer.Console.Endpoint.Run(ctx)
	})

	return group.Wait()
}

// Close closes all the resources.
func (peer *Peer) Close() error {
	peer.Log.Debug("mempool closing")
	var errlist errs.Group

	if peer.Console.Endpoint != nil {
		errlist.Add(peer.Console.Endpoint.Close())
	}

	if err := errlist.Err(); err != nil {
		peer.Log.Error("could not close mempool", err)
		return err
	}

	return nil
}
