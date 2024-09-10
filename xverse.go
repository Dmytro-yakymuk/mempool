package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/jackc/pgx"
)

type RuneTransaction struct {
	TxID           string `json:"txid"`
	Amount         string `json:"amount"`
	BlockHeight    int    `json:"blockHeight"`
	BlockTimestamp string `json:"blockTimestamp"`
	Burned         bool   `json:"burned"`
}

type RuneResponse struct {
	Items        []RuneTransaction `json:"items"`
	Divisibility int               `json:"divisibility"`
	RuneName     string            `json:"runeName"`
	Total        int               `json:"total"`
	Offset       int               `json:"offset"`
	Limit        int               `json:"limit"`
}

// GET https://api-3.xverse.app/v1/address/bc1ptwm0lteuht2ulyatwaakqcmfwy3td4nevtp5uqnqjx8ukwr8am3srzdmpn/rune/IS%E2%80%A2THIS%E2%80%A2WORKING?offset=0&limit=50
//
//	{
//	    "items": [
//	        {
//	            "txid": "3725130fd177341424217c5a0438a5e913b7fff0714c7b7d7537c2694dbb68c5",
//	            "amount": "1000",
//	            "blockHeight": 860585,
//	            "blockTimestamp": "2024-09-09T09:27:26.000Z",
//	            "burned": false
//	        }
//	    ],
//	    "divisibility": 0,
//	    "runeName": "IS•THIS•WORKING",
//	    "total": 1,
//	    "offset": 0,
//	    "limit": 50
//	}
func HandleGetRuneTransactions(w http.ResponseWriter, r *http.Request, conn *pgx.Conn) {
	vars := mux.Vars(r)
	address := vars["address"]
	rune := vars["rune"]

	queryParams := r.URL.Query()
	offset, err := strconv.Atoi(queryParams.Get("offset"))
	if err != nil {
		offset = 0
	}
	limit, err := strconv.Atoi(queryParams.Get("limit"))
	if err != nil {
		limit = 50
	}

	// Get total count
	var total int
	err = conn.QueryRow(
		`SELECT COUNT(*)
		FROM runes_utxos
		WHERE address = $1 AND rune = $2`,
		address, rune).Scan(&total)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rows, err := conn.QueryEx(context.Background(),
		`SELECT runes_utxos.amount, runes_utxos.tx_hash as txid, runes_utxos.block as blockHeight, runes_utxos.spend
		FROM runes
		JOIN runes_utxos ON runes.rune = runes_utxos.rune
		WHERE runes_utxos.address = $1 AND runes_utxos.rune = $2
		LIMIT $3 OFFSET $4
		`,
		&pgx.QueryExOptions{}, address, rune, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var runeResponse RuneResponse
	runeResponse.Items = make([]RuneTransaction, 0, 10)
	runeResponse.Total = total
	runeResponse.Offset = offset
	runeResponse.Limit = limit

	// handle rows
	for rows.Next() {
		var spend bool
		var txid string
		var amount, blockHeight int
		err := rows.Scan(&amount, &txid, &blockHeight, &spend)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		runeResponse.Items = append(runeResponse.Items, RuneTransaction{
			TxID:           txid,
			Amount:         strconv.Itoa(amount),
			BlockHeight:    blockHeight,
			BlockTimestamp: "2024-09-09T09:27:26.000Z",
			Burned:         false,
		})
	}

	// Response
	responseBytes, err := json.Marshal(runeResponse)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(responseBytes)
}

func HandleRuneBalanceRequest(w http.ResponseWriter, r *http.Request, conn *pgx.Conn) {
	vars := mux.Vars(r)
	address := vars["address"]

	var tx_hash, rune, symbol string
	var block, tx_id, divisibility, amount int
	rows, err := conn.QueryEx(context.Background(), `SELECT r.rune, ru.divisibility, ru.symbol, ru.block, ru.tx_id, SUM(r.amount) AS amount
		FROM runes_utxos r
		JOIN runes ru ON r.rune = ru.rune
		WHERE r.spend = false AND r.address = $1
		GROUP BY r.rune, ru.divisibility, ru.symbol, ru.block, ru.tx_id
		ORDER BY amount DESC`, &pgx.QueryExOptions{}, address)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var runeBalances []RuneBalance = make([]RuneBalance, 0, 10)

	// handle rows
	for rows.Next() {
		err := rows.Scan(&rune, &divisibility, &symbol, &block, &tx_id, &amount)
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
