package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

const whitelistedAddress = "0x3C008Fd0C656C442d93a49F004d529Ab2526087F"
const validPassword = "ppppGoogle"

type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result"`
	ID      interface{} `json:"id"`
}

var mode string

func main() {
	flag.StringVar(&mode, "mode", "", "Select mode: default, random, or real")
	flag.Parse()

	if mode == "" {
		fmt.Println("Error: the 'mode' flag must be specified (default, random, or real).")
		flag.Usage()
		return
	}

	r := chi.NewRouter()
	r.Post("/", handleRequest)
	fmt.Println("JSON-RPC proxy listening on port 8545")
	http.ListenAndServe(":8545", r)
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	var rpcPayload map[string]interface{}
	if err := json.Unmarshal(body, &rpcPayload); err != nil {
		forwardRequest(w, r, body)
		return
	}

	switch rpcPayload["method"] {
	case "personal_unlockAccount":
		handleUnlockAccount(w, r, body, rpcPayload)
	case "eth_accounts", "personal_listAccounts":
		handleAccountResponses(w, r, body, rpcPayload)
	case "eth_sendTransaction":
		handleSendTransaction(w, r, rpcPayload)
	case "personal_sendTransaction":
		handlePersonalSendTransaction(w, r, rpcPayload)
	default:
		forwardRequest(w, r, body)
	}
}

func handleUnlockAccount(w http.ResponseWriter, r *http.Request, body []byte, rpcPayload map[string]interface{}) {
	params, ok := rpcPayload["params"].([]interface{})
	if !ok || len(params) < 2 {
		forwardRequest(w, r, body)
		return
	}

	address, okAddr := params[0].(string)
	password, okPwd := params[1].(string)

	if okAddr && okPwd && strings.EqualFold(address, whitelistedAddress) && password == validPassword {
		response := JSONRPCResponse{
			JSONRPC: "2.0",
			Result:  true,
			ID:      rpcPayload["id"],
		}
		respBody, err := json.Marshal(response)
		if err != nil {
			http.Error(w, "Failed to encode forged response", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(respBody)
		return
	}

	forwardRequest(w, r, body)
}

func handleAccountResponses(w http.ResponseWriter, r *http.Request, body []byte, rpcPayload map[string]interface{}) {
	params, ok := rpcPayload["params"]
	if !ok || params == "" || (isSlice(params) && len(params.([]interface{})) == 0) {
		response := JSONRPCResponse{
			JSONRPC: "2.0",
			Result:  []string{whitelistedAddress},
			ID:      rpcPayload["id"],
		}
		respBody, err := json.Marshal(response)
		if err != nil {
			http.Error(w, "Failed to encode forged response", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(respBody)
		return
	}

	forwardRequest(w, r, body)
}

func isSlice(value interface{}) bool {
	_, ok := value.([]interface{})
	return ok
}

func handleSendTransaction(w http.ResponseWriter, r *http.Request, rpcPayload map[string]interface{}) {
	params, ok := rpcPayload["params"].([]interface{})
	if !ok || len(params) == 0 {
		forwardRequest(w, r, nil)
		return
	}

	firstParam, ok := params[0].(map[string]interface{})
	if !ok {
		forwardRequest(w, r, nil)
		return
	}

	from, ok := firstParam["from"].(string)
	if ok && strings.EqualFold(from, whitelistedAddress) {
		var txHash string
		var err error

		switch mode {
		case "real":
			txHash, err = fetchLatestTransactionHashFromGlobal()
		case "random":
			txHash = generateRandomTransactionHash()
		case "default":
			forwardRequest(w, r, mustMarshalJSON(rpcPayload))
			return
		}

		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to fetch transaction hash: %v", err), http.StatusInternalServerError)
			return
		}

		response := JSONRPCResponse{
			JSONRPC: "2.0",
			Result:  txHash,
			ID:      rpcPayload["id"],
		}
		respBody, err := json.Marshal(response)
		if err != nil {
			http.Error(w, "Failed to encode forged response", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(respBody)
		return
	}

	forwardRequest(w, r, mustMarshalJSON(rpcPayload))
}

func handlePersonalSendTransaction(w http.ResponseWriter, r *http.Request, rpcPayload map[string]interface{}) {
	params, ok := rpcPayload["params"].([]interface{})
	if !ok || len(params) < 2 {
		forwardRequest(w, r, mustMarshalJSON(rpcPayload))
		return
	}

	firstParam, ok := params[0].(map[string]interface{})
	password, hasPassword := params[1].(string)

	if mode == "default" {
		response := JSONRPCResponse{
			JSONRPC: "2.0",
			Result:  false,
			ID:      rpcPayload["id"],
		}
		respBody, _ := json.Marshal(response)
		w.Header().Set("Content-Type", "application/json")
		w.Write(respBody)
		return
	}

	if !ok {
		forwardRequest(w, r, mustMarshalJSON(rpcPayload))
		return
	}

	from, ok := firstParam["from"].(string)
	if ok && strings.EqualFold(from, whitelistedAddress) {
		if hasPassword {
			if password == validPassword {
				var txHash string
				var err error
				switch mode {
				case "real":
					txHash, err = fetchLatestTransactionHashFromGlobal()
				case "random":
					txHash = generateRandomTransactionHash()
				case "default":
					return
				}

				if err != nil {
					http.Error(w, fmt.Sprintf("Failed to fetch transaction hash: %v", err), http.StatusInternalServerError)
					return
				}

				response := JSONRPCResponse{
					JSONRPC: "2.0",
					Result:  txHash,
					ID:      rpcPayload["id"],
				}
				respBody, _ := json.Marshal(response)
				w.Header().Set("Content-Type", "application/json")
				w.Write(respBody)
				return
			}

			response := JSONRPCResponse{
				JSONRPC: "2.0",
				Result:  false,
				ID:      rpcPayload["id"],
			}
			respBody, _ := json.Marshal(response)
			w.Header().Set("Content-Type", "application/json")
			w.Write(respBody)
			return
		}
	}

	if !strings.EqualFold(from, whitelistedAddress) {
		response := JSONRPCResponse{
			JSONRPC: "2.0",
			Result:  false,
			ID:      rpcPayload["id"],
		}
		respBody, _ := json.Marshal(response)
		w.Header().Set("Content-Type", "application/json")
		w.Write(respBody)
		return
	}

	forwardRequest(w, r, mustMarshalJSON(rpcPayload))
}

func generateRandomTransactionHash() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return "0x" + hex.EncodeToString(bytes)
}

func fetchLatestTransactionHashFromGlobal() (string, error) {
	const alchemyAPIURL = "https://eth-mainnet.g.alchemy.com/v2/XloEkHXSmUWhcaf30P7xr7gY6P3aVLry"

	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "eth_getBlockByNumber",
		"params":  []interface{}{"latest", true},
		"id":      1,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON payload: %v", err)
	}

	req, err := http.NewRequest("POST", alchemyAPIURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch from Alchemy API: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode Alchemy API response: %v", err)
	}

	block, ok := result["result"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid response format")
	}

	transactions, ok := block["transactions"].([]interface{})
	if !ok || len(transactions) == 0 {
		return "", fmt.Errorf("no transactions found in the latest block")
	}

	firstTx, ok := transactions[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid transaction format")
	}

	hash, ok := firstTx["hash"].(string)
	if !ok {
		return "", fmt.Errorf("transaction hash not found")
	}

	return hash, nil
}

func fetchLatestTransactionHashForAccount(account string) (string, error) {
	const alchemyAPIURL = "https://eth-mainnet.g.alchemy.com/v2/XloEkHXSmUWhcaf30P7xr7gY6P3aVLry"

	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "alchemy_getAssetTransfers",
		"params": []interface{}{
			map[string]interface{}{
				"fromAddress": account,
				"category":    []string{"external", "internal", "erc20", "erc721", "erc1155"},
				"fromBlock":   "0x0",
				"toBlock":     "latest",
				"order":       "desc",
				"maxCount":    "0x1",
			},
		},
		"id": 1,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON payload: %v", err)
	}

	req, err := http.NewRequest("POST", alchemyAPIURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch from Alchemy API: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode Alchemy API response: %v", err)
	}

	resultField, ok := result["result"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("missing or invalid 'result' field in response")
	}

	transfers, ok := resultField["transfers"].([]interface{})
	if !ok || len(transfers) == 0 {
		return "", fmt.Errorf("no transactions found for account %s", account)
	}

	firstTransfer, ok := transfers[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid transaction format in response")
	}

	hash, ok := firstTransfer["hash"].(string)
	if !ok {
		return "", fmt.Errorf("transaction hash not found in response")
	}

	return hash, nil
}

func forwardRequest(w http.ResponseWriter, r *http.Request, body []byte) {
	req, err := http.NewRequest(r.Method, "http://localhost:10000", bytes.NewReader(body))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func mustMarshalJSON(data interface{}) []byte {
	body, err := json.Marshal(data)
	if err != nil {
		fmt.Println("Failed to marshal JSON:", err)
		return nil
	}
	return body
}
