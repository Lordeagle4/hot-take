// Package examplemcp provides a deliberately small, read-only MCP fixture.
// It implements only the tools/list and tools/call subset used by the example.
package examplemcp

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/Lordeagle4/hot-take/mcp"
)

// Handler serves a fixed catalogue. FailCalls injects an HTTP failure for the
// documented failure exercise; it must not be changed while serving requests.
type Handler struct{ FailCalls bool }

type requestMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// ServeHTTP handles the example's stateless MCP subset without external I/O.
func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/mcp" {
		http.Error(w, "POST /mcp required", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("Origin") != "" {
		http.Error(w, "browser origins are not accepted", http.StatusForbidden)
		return
	}
	if r.Header.Get("MCP-Protocol-Version") != mcp.ProtocolVersion {
		http.Error(w, "unsupported protocol version", http.StatusBadRequest)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var request requestMessage
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if request.JSONRPC != "2.0" || len(request.ID) == 0 {
		http.Error(w, "JSON-RPC request ID required", http.StatusBadRequest)
		return
	}
	var result json.RawMessage
	switch request.Method {
	case "tools/list":
		result = json.RawMessage(`{"resultType":"complete","tools":[{"name":"catalogue.lookup","description":"Look up the example product by SKU.","inputSchema":{"type":"object","properties":{"sku":{"type":"string"}},"required":["sku"],"additionalProperties":false}}]}`)
	case "tools/call":
		if h.FailCalls {
			http.Error(w, "intentional example failure", http.StatusServiceUnavailable)
			return
		}
		var params struct {
			Name      string `json:"name"`
			Arguments struct {
				SKU string `json:"sku"`
			} `json:"arguments"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil || params.Name != "catalogue.lookup" {
			http.Error(w, "invalid tool call", http.StatusBadRequest)
			return
		}
		if params.Arguments.SKU != "DEMO-001" {
			result = json.RawMessage(`{"resultType":"complete","isError":true,"content":[{"type":"text","text":"Unknown SKU. Use DEMO-001."}]}`)
		} else {
			result = json.RawMessage(`{"resultType":"complete","content":[{"type":"text","text":"DEMO-001: Field Notebook, 12 units in stock."}]}`)
		}
	default:
		http.Error(w, "unsupported example method", http.StatusBadRequest)
		return
	}
	payload, err := json.Marshal(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  json.RawMessage `json:"result"`
	}{"2.0", request.ID, result})
	if err != nil {
		http.Error(w, "encode response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := io.WriteString(w, string(payload)); err != nil {
		log.Printf("write example MCP response: %v", err)
	}
}
