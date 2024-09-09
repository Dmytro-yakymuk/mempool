package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/jackc/pgx"
)

var (
	host   string
	port   int
	btcrpc string
	pgurl  string
)

type RPCRequest struct {
	JsonRPC string        `json:"jsonrpc"`
	ID      string        `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

type RPCResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	ID string `json:"id"`
}

type UTXO struct {
	Txid   string `json:"txid"`
	Vout   int    `json:"vout"`
	Status struct {
		Confirmed   bool   `json:"confirmed"`
		BlockHeight int    `json:"block_height"`
		BlockHash   string `json:"block_hash"`
		BlockTime   int64  `json:"block_time"`
	} `json:"status"`
	Value int64 `json:"value"`
}

type RuneBalance struct {
	Amount        int     `json:"amount"`
	Divisibility  int     `json:"divisibility"`
	Symbol        *string `json:"symbol"`
	RuneName      string  `json:"runeName"`
	InscriptionId string  `json:"inscriptionId"`
	ID            string  `json:"id"`
}

func init() {
	flag.StringVar(&host, "host", "0.0.0.0", "Host to listen on")
	flag.IntVar(&port, "port", 12345, "Port to listen on")
	flag.StringVar(&btcrpc, "btcrpc", "http://user:password@localhost:48000", "Bitcoin RPC URL")
	flag.StringVar(&pgurl, "pgurl", "postgres://dev:dev@127.0.0.1:5432/runes_dex", "Postgres URL")
}

func main() {
	flag.Parse()

	if err := checkBitcoinConnection(); err != nil {
		log.Fatalf("Failed to connect to Bitcoin node: %v", err)
	}

	conf, err := ParsePgConn(pgurl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to parse Postgres connection URL: %v\n", err)
		os.Exit(1)
	}

	// open a connection to the database
	conn, err := pgx.Connect(*conf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to database: %v\n", err)
		os.Exit(1)
	}
	log.Printf("Database connection OK")
	defer conn.Close()

	r := mux.NewRouter()
	r.Use(loggingMiddleware)
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		dbStatus := "error"
		if conn.IsAlive() {
			dbStatus = "ok"
		}

		btcStatus := "error"

		if err := checkBitcoinConnection(); err == nil {
			btcStatus = "ok"
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "ok",
			"db":     dbStatus,
			"btc":    btcStatus,
		})
	})
	r.HandleFunc("/api/v1/address/{address}/utxo", handleUTXORequest)
	r.HandleFunc("/api/v2/address/{address}/rune-balance", func(w http.ResponseWriter, r *http.Request) {
		handleRuneBalanceRequest(w, r, conn)
	})

	addr := fmt.Sprintf("%s:%d", host, port)
	log.Printf("Server listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, r))
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL)
		next.ServeHTTP(w, r)
	})
}

func checkBitcoinConnection() error {
	_, err := callRPC("getblockchaininfo", []interface{}{})
	if err != nil {
		return fmt.Errorf("failed to connect to Bitcoin node: %v", err)
	}
	log.Println("Successfully connected to Bitcoin node")
	return nil
}

func handleUTXORequest(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	address := vars["address"]

	utxos, err := getUTXOs(address)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(utxos)
}

func handleRuneBalanceRequest(w http.ResponseWriter, r *http.Request, conn *pgx.Conn) {
	vars := mux.Vars(r)
	address := vars["address"]

	var tx_hash, rune, symbol string
	var block, tx_id, output_n, divisibility, amount int
	rows, err := conn.Query(`SELECT r.block, r.tx_id, r.tx_hash, r.output_n, r.rune, ru.divisibility, ru.symbol, r.amount
		FROM runes_utxos r
		JOIN runes ru ON r.rune = ru.rune
		WHERE r.spend = false AND r.address = $1
		ORDER BY r.amount DESC`, address)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var runeBalances []RuneBalance

	// handle rows
	for rows.Next() {
		err := rows.Scan(&block, &tx_id, &tx_hash, &output_n, &rune, &divisibility, &symbol, &amount)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		runeBalance := RuneBalance{
			ID:            fmt.Sprintf("%d:%d", block, tx_id),
			Amount:        amount,
			RuneName:      rune,
			Divisibility:  divisibility,
			Symbol:        &symbol,
			InscriptionId: tx_hash,
		}

		runeBalances = append(runeBalances, runeBalance)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(runeBalances)
}

func getUTXOs(address string) ([]UTXO, error) {
	scanObjects := []string{fmt.Sprintf("addr(%s)", address)}
	result, err := callRPC("scantxoutset", []interface{}{"start", scanObjects})
	if err != nil {
		return nil, fmt.Errorf("error scanning txoutset: %v", err)
	}

	var scanResp struct {
		Unspents []struct {
			Txid   string  `json:"txid"`
			Vout   int     `json:"vout"`
			Amount float64 `json:"amount"`
		} `json:"unspents"`
	}
	if err := json.Unmarshal(result, &scanResp); err != nil {
		return nil, fmt.Errorf("error unmarshalling scan response: %v", err)
	}

	var utxos []UTXO
	for _, unspent := range scanResp.Unspents {
		utxo := UTXO{
			Txid:  unspent.Txid,
			Vout:  unspent.Vout,
			Value: int64(unspent.Amount * 1e8), // Convert BTC to satoshis
		}

		// Get transaction details to fill in the status
		txResult, err := callRPC("getrawtransaction", []interface{}{unspent.Txid, true})
		if err != nil {
			return nil, fmt.Errorf("error getting transaction details: %v", err)
		}

		var txInfo struct {
			BlockHash     string `json:"blockhash"`
			BlockTime     int64  `json:"blocktime"`
			Confirmations int    `json:"confirmations"`
		}
		if err := json.Unmarshal(txResult, &txInfo); err != nil {
			return nil, fmt.Errorf("error unmarshalling transaction info: %v", err)
		}

		utxo.Status.Confirmed = txInfo.Confirmations > 0
		utxo.Status.BlockHash = txInfo.BlockHash
		utxo.Status.BlockTime = txInfo.BlockTime

		if txInfo.Confirmations > 0 {
			blockResult, err := callRPC("getblock", []interface{}{txInfo.BlockHash})
			if err != nil {
				return nil, fmt.Errorf("error getting block details: %v", err)
			}

			var blockInfo struct {
				Height int `json:"height"`
			}
			if err := json.Unmarshal(blockResult, &blockInfo); err != nil {
				return nil, fmt.Errorf("error unmarshalling block info: %v", err)
			}

			utxo.Status.BlockHeight = blockInfo.Height
		}

		utxos = append(utxos, utxo)
	}

	return utxos, nil
}

func callRPC(method string, params []interface{}) (json.RawMessage, error) {
	request := RPCRequest{
		JsonRPC: "1.0",
		ID:      "go-bitcoin-rpc",
		Method:  method,
		Params:  params,
	}

	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("error marshalling request: %v", err)
	}

	resp, err := http.Post(btcrpc, "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	var rpcResp RPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, fmt.Errorf("error decoding response: %v", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("RPC error (code %d): %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return rpcResp.Result, nil
}
