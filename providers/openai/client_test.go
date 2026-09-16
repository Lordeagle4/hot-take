package openai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Lordeagle4/hot-take/provider"
	"github.com/Lordeagle4/hot-take/tool"
)

func TestClientContinuesToolCallWithPreviousResponse(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Authorization = %q", request.Header.Get("Authorization"))
		}
		var body responseRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			response.WriteHeader(http.StatusBadRequest)
			return
		}

		switch requests.Add(1) {
		case 1:
			if body.PreviousResponseID != "" || len(body.Input) != 1 || body.Input[0].Role != provider.User {
				t.Errorf("first request = %#v", body)
			}
			response.Header().Set("Content-Type", "application/json")
			writeResponse(t, response, `{"id":"resp_1","output":[{"type":"reasoning"},{"type":"function_call","call_id":"call_1","name":"clock_now","arguments":"{}"}]}`)
		case 2:
			if body.PreviousResponseID != "resp_1" {
				t.Errorf("PreviousResponseID = %q", body.PreviousResponseID)
			}
			if len(body.Input) != 1 || body.Input[0].Type != "function_call_output" || body.Input[0].CallID != "call_1" {
				t.Errorf("second request input = %#v", body.Input)
			}
			response.Header().Set("Content-Type", "application/json")
			writeResponse(t, response, `{"id":"resp_2","output":[{"type":"message","content":[{"type":"output_text","text":"It is noon UTC."}]}]}`)
		default:
			t.Errorf("unexpected request")
			response.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client, err := New(Config{APIKey: "test-key", Model: "test-model", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	definition := tool.Definition{
		Name:        "clock_now",
		Description: "Return the current time.",
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
		Strict:      true,
	}
	first, err := client.Generate(context.Background(), provider.Request{
		Instructions: "Use the clock.",
		Items:        []provider.Item{{Message: &provider.Message{Role: provider.User, Content: "What time is it?"}}},
		Tools:        []tool.Definition{definition},
	})
	if err != nil {
		t.Fatalf("first Generate() error = %v", err)
	}
	if len(first.ToolCalls) != 1 || first.ToolCalls[0].ID != "call_1" {
		t.Fatalf("first Generate() calls = %#v", first.ToolCalls)
	}

	call := first.ToolCalls[0]
	result := tool.Result{CallID: call.ID, Content: json.RawMessage(`{"time":"2026-09-16T12:00:00Z"}`)}
	second, err := client.Generate(context.Background(), provider.Request{
		Instructions: "Use the clock.",
		Items: []provider.Item{
			{Message: &provider.Message{Role: provider.User, Content: "What time is it?"}},
			{ToolCall: &call},
			{ToolResult: &result},
		},
		Tools: []tool.Definition{definition},
		State: first.State,
	})
	if err != nil {
		t.Fatalf("second Generate() error = %v", err)
	}
	if second.Text != "It is noon UTC." {
		t.Fatalf("second Generate() text = %q", second.Text)
	}
	if requests.Load() != 2 {
		t.Fatalf("request count = %d", requests.Load())
	}
}

func TestClientReturnsStructuredAPIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("x-request-id", "req_test")
		response.WriteHeader(http.StatusTooManyRequests)
		writeResponse(t, response, `{"error":{"message":"rate limit exceeded","type":"rate_limit_error"}}`)
	}))
	defer server.Close()

	client, err := New(Config{APIKey: "test-key", Model: "test-model", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = client.Generate(context.Background(), provider.Request{Items: []provider.Item{{Message: &provider.Message{Role: provider.User, Content: "Hello"}}}})
	if !errors.Is(err, ErrAPI) || !strings.Contains(err.Error(), "req_test") {
		t.Fatalf("Generate() error = %v", err)
	}
}

func TestNewRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()

	tests := map[string]Config{
		"missing API key": {Model: "test-model"},
		"unsafe HTTP":    {APIKey: "test-key", Model: "test-model", BaseURL: "http://api.example.com/v1"},
		"URL credentials": {APIKey: "test-key", Model: "test-model", BaseURL: "https://user@example.com/v1"},
		"URL query":       {APIKey: "test-key", Model: "test-model", BaseURL: "https://api.example.com/v1?debug=true"},
	}
	for name, config := range tests {
		config := config
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := New(config)
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("New() error = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

func writeResponse(t *testing.T, response http.ResponseWriter, payload string) {
	t.Helper()
	if _, err := response.Write([]byte(payload)); err != nil {
		t.Errorf("write response: %v", err)
	}
}
