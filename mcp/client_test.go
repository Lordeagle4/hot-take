package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Lordeagle4/hot-take/mcp"
)

func TestClientListsAndCallsTools(t *testing.T) {
	t.Parallel()

	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()
		if request.Header.Get("MCP-Protocol-Version") != mcp.ProtocolVersion {
			t.Errorf("MCP-Protocol-Version = %q", request.Header.Get("MCP-Protocol-Version"))
		}
		if request.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("Authorization = %q", request.Header.Get("Authorization"))
		}
		var message struct {
			ID     int64  `json:"id"`
			Method string `json:"method"`
			Params struct {
				Name string `json:"name"`
				Meta struct {
					Version string `json:"io.modelcontextprotocol/protocolVersion"`
				} `json:"_meta"`
			} `json:"params"`
		}
		if err := json.NewDecoder(request.Body).Decode(&message); err != nil {
			t.Errorf("Decode() error = %v", err)
			return
		}
		if message.Params.Meta.Version != mcp.ProtocolVersion {
			t.Errorf("body protocol version = %q", message.Params.Meta.Version)
		}
		writer.Header().Set("Content-Type", "application/json")
		switch message.Method {
		case "tools/list":
			requests.Add(1)
			fmt.Fprintf(writer, `{"jsonrpc":"2.0","id":%d,"result":{"resultType":"complete","tools":[{"name":"search.items","description":"Search items.","inputSchema":{"type":"object","properties":{"query":{"type":"string","x-mcp-header":"Query"}}}}]}}`, message.ID)
		case "tools/call":
			requests.Add(1)
			if request.Header.Get("Mcp-Method") != "tools/call" || request.Header.Get("Mcp-Name") != "search.items" {
				t.Errorf("MCP routing headers = %q, %q", request.Header.Get("Mcp-Method"), request.Header.Get("Mcp-Name"))
			}
			if request.Header.Get("Mcp-Param-Query") != "=?base64?SGVsbG8sIOS4lueVjA==?=" {
				t.Errorf("Mcp-Param-Query = %q", request.Header.Get("Mcp-Param-Query"))
			}
			fmt.Fprintf(writer, `{"jsonrpc":"2.0","id":%d,"result":{"resultType":"complete","content":[{"type":"text","text":"found"}],"isError":false}}`, message.ID)
		default:
			t.Errorf("method = %q", message.Method)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server, "secret")
	definitions, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(definitions) != 1 || definitions[0].Name != "search.items" {
		t.Fatalf("ListTools() = %#v", definitions)
	}
	result, err := client.CallTool(context.Background(), "search.items", json.RawMessage(`{"query":"Hello, 世界"}`))
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if !strings.Contains(string(result), `"text":"found"`) {
		t.Fatalf("CallTool() = %s", result)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want 2", requests.Load())
	}
}

func TestClientSupportsSSEAndPagination(t *testing.T) {
	t.Parallel()

	var page atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()
		var message struct {
			ID int64 `json:"id"`
		}
		if err := json.NewDecoder(request.Body).Decode(&message); err != nil {
			t.Errorf("Decode() error = %v", err)
			return
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(writer, ": keepalive\n\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\"}\n\n")
		if page.Add(1) == 1 {
			fmt.Fprintf(writer, "data: {\"jsonrpc\":\"2.0\",\"id\":%d,\"result\":{\"resultType\":\"complete\",\"tools\":[{\"name\":\"first\",\"description\":\"First.\",\"inputSchema\":{\"type\":\"object\"}}],\"nextCursor\":\"next\"}}\n\n", message.ID)
			return
		}
		fmt.Fprintf(writer, "data: {\"jsonrpc\":\"2.0\",\"id\":%d,\"result\":{\"resultType\":\"complete\",\"tools\":[{\"name\":\"second\",\"description\":\"Second.\",\"inputSchema\":{\"type\":\"object\"}}]}}\n\n", message.ID)
	}))
	defer server.Close()

	definitions, err := newTestClient(t, server, "").ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(definitions) != 2 || definitions[0].Name != "first" || definitions[1].Name != "second" {
		t.Fatalf("ListTools() = %#v", definitions)
	}
}

func TestClientExcludesInvalidHeaderTool(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()
		var message struct {
			ID int64 `json:"id"`
		}
		if err := json.NewDecoder(request.Body).Decode(&message); err != nil {
			t.Errorf("Decode() error = %v", err)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(writer, `{"jsonrpc":"2.0","id":%d,"result":{"resultType":"complete","tools":[{"name":"bad","description":"Bad.","inputSchema":{"type":"object","items":{"x-mcp-header":"Bad","type":"string"}}},{"name":"good","description":"Good.","inputSchema":{"type":"object"}}]}}`, message.ID)
	}))
	defer server.Close()

	definitions, err := newTestClient(t, server, "").ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(definitions) != 1 || definitions[0].Name != "good" {
		t.Fatalf("ListTools() = %#v", definitions)
	}
}

func TestNewRejectsInsecureRemoteEndpoint(t *testing.T) {
	t.Parallel()

	_, err := mcp.New(mcp.Config{
		Endpoint:      "http://example.com/mcp",
		ClientName:    "test",
		ClientVersion: "1",
		HTTPClient:    &http.Client{Timeout: time.Second},
	})
	if err == nil {
		t.Fatal("New() error = nil, want error")
	}
}

func newTestClient(t *testing.T, server *httptest.Server, token string) *mcp.Client {
	t.Helper()
	client, err := mcp.New(mcp.Config{
		Endpoint:      server.URL,
		BearerToken:   token,
		ClientName:    "hot-take-test",
		ClientVersion: "0",
		HTTPClient:    &http.Client{Timeout: 2 * time.Second},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return client
}
