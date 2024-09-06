package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

var (
	host   string
	port   int
	btcrpc string
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

func init() {
	flag.StringVar(&host, "host", "0.0.0.0", "Host to listen on")
	flag.IntVar(&port, "port", 12345, "Port to listen on")
	flag.StringVar(&btcrpc, "btcrpc", "http://user:password@localhost:8332", "Bitcoin RPC URL")
}

func main() {
	flag.Parse()

	if err := checkBitcoinConnection(); err != nil {
		log.Fatalf("Failed to connect to Bitcoin node: %v", err)
	}

	r := mux.NewRouter()
	r.HandleFunc("/api/v1/address/{address}/utxo", handleUTXORequest)

	addr := fmt.Sprintf("%s:%d", host, port)
	log.Printf("Server listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, r))
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
