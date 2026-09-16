package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Lordeagle4/hot-take/agent"
	"github.com/Lordeagle4/hot-take/internal/examplemcp"
	"github.com/Lordeagle4/hot-take/permission"
	"github.com/Lordeagle4/hot-take/project"
	"github.com/Lordeagle4/hot-take/provider"
	"github.com/Lordeagle4/hot-take/providers/openai"
	"github.com/Lordeagle4/hot-take/tool"
)

func TestMCPIntegration(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		approval   string
		fail       bool
		wantCalls  int32
		wantModels int32
	}{
		{"approved", "yes\n", false, 1, 2},
		{"denied", "no\n", false, 0, 1},
		{"server failure", "yes\n", true, 1, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var calls, models atomic.Int32
			fixture := examplemcp.Handler{FailCalls: test.fail}
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("MCP-Method") == "tools/call" {
					calls.Add(1)
				}
				fixture.ServeHTTP(w, r)
			}))
			defer remote.Close()
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					PreviousResponseID string `json:"previous_response_id"`
					Input              []struct {
						Type   string `json:"type"`
						CallID string `json:"call_id"`
						Output string `json:"output"`
					} `json:"input"`
					Tools []struct {
						Name string `json:"name"`
					} `json:"tools"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					http.Error(w, "bad request", 400)
					return
				}
				if len(body.Tools) != 1 || body.Tools[0].Name != "catalogue__catalogue_lookup" {
					t.Errorf("tools = %#v", body.Tools)
				}
				w.Header().Set("Content-Type", "application/json")
				var payload string
				switch models.Add(1) {
				case 1:
					payload = `{"id":"resp_1","output":[{"type":"function_call","call_id":"lookup-1","name":"catalogue__catalogue_lookup","arguments":"{\"sku\":\"DEMO-001\"}"}]}`
				case 2:
					if body.PreviousResponseID != "resp_1" || len(body.Input) != 1 || body.Input[0].CallID != "lookup-1" || body.Input[0].Type != "function_call_output" || !strings.Contains(body.Input[0].Output, "12 units") {
						t.Errorf("continuation = %#v", body)
					}
					payload = `{"id":"resp_2","output":[{"type":"message","content":[{"type":"output_text","text":"Field Notebook: 12 units in stock."}]}]}`
				default:
					t.Error("unexpected model request")
					http.Error(w, "unexpected request", 500)
					return
				}
				if _, err := io.WriteString(w, payload); err != nil {
					t.Error(err)
				}
			}))
			defer api.Close()
			model, err := openai.New(openai.Config{APIKey: "test-key", Model: "test-model", BaseURL: api.URL, HTTPClient: api.Client()})
			if err != nil {
				t.Fatal(err)
			}
			runtime := integrationRuntime(t, remote.URL+"/mcp", test.approval, model)
			answer, err := runtime.Run(context.Background(), "How many DEMO-001 are in stock?")
			switch test.name {
			case "approved":
				if err != nil || !strings.Contains(answer, "12 units") {
					t.Fatalf("answer %q, error %v", answer, err)
				}
			case "denied":
				if !errors.Is(err, permission.ErrDenied) {
					t.Fatalf("error = %v", err)
				}
			case "server failure":
				if err == nil || !strings.Contains(err.Error(), "503") {
					t.Fatalf("error = %v", err)
				}
			}
			if calls.Load() != test.wantCalls || models.Load() != test.wantModels {
				t.Fatalf("tool calls %d, model calls %d", calls.Load(), models.Load())
			}
		})
	}
}

func integrationRuntime(t *testing.T, endpoint string, approval string, model provider.Model) *agent.Runtime {
	t.Helper()
	definition, err := (project.Loader{}).Load("../../examples/mcp-agent")
	if err != nil {
		t.Fatal(err)
	}
	definition.Plugins[0].Endpoint = endpoint
	tools, capabilities, policy, err := projectTools(context.Background(), definition, strings.NewReader(approval), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := agent.NewRuntime(agent.Config{Name: definition.Name, Instructions: definition.Instructions, MaxSteps: definition.MaxSteps, MaxToolCalls: 2, RunTimeout: time.Minute, Skills: definition.Skills, Tools: tools, Capabilities: capabilities, Authorizer: policy, Model: model})
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}

// TestLiveMCPIntegration is opt-in because it makes billable model requests.
// The local read-only tool is pre-approved only inside this test harness.
func TestLiveMCPIntegration(t *testing.T) {
	modelName := os.Getenv("HOT_TAKE_LIVE_MODEL")
	if modelName == "" {
		t.Skip("set HOT_TAKE_LIVE_MODEL and OPENAI_API_KEY to run the live smoke test")
	}
	var calls atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("MCP-Method") == "tools/call" {
			calls.Add(1)
		}
		examplemcp.Handler{}.ServeHTTP(w, r)
	}))
	defer remote.Close()
	model, err := openai.New(openai.Config{APIKey: os.Getenv("OPENAI_API_KEY"), Model: modelName})
	if err != nil {
		t.Fatal(err)
	}
	runtime := integrationRuntime(t, remote.URL+"/mcp", "yes\nyes\n", model)
	answer, err := runtime.Run(context.Background(), "Use the catalogue tool to look up DEMO-001. Report its product name and stock count as digits.")
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() < 1 || !strings.Contains(answer, "12") || !strings.Contains(strings.ToLower(answer), "notebook") {
		t.Fatalf("calls %d, answer %q", calls.Load(), answer)
	}
}

func TestMCPDiscoveryDeadline(t *testing.T) {
	t.Parallel()
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Error(err)
			return
		}
		<-r.Context().Done()
	}))
	defer remote.Close()
	definition, err := (project.Loader{}).Load("../../examples/mcp-agent")
	if err != nil {
		t.Fatal(err)
	}
	definition.Plugins[0].Endpoint = remote.URL + "/mcp"
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, _, _, err = projectTools(ctx, definition, strings.NewReader(""), io.Discard)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v", err)
	}
}

type catalogueModel struct{}

func (catalogueModel) Generate(context.Context, provider.Request) (provider.Turn, error) {
	return provider.Turn{ToolCalls: []tool.Call{{ID: "lookup", Name: "catalogue__catalogue_lookup", Arguments: json.RawMessage(`{"sku":"DEMO-001"}`)}}}, nil
}

func TestMCPCallCancellation(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("MCP-Method") != "tools/call" {
			examplemcp.Handler{}.ServeHTTP(w, r)
			return
		}
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Error(err)
			return
		}
		close(started)
		<-r.Context().Done()
	}))
	defer remote.Close()
	runtime := integrationRuntime(t, remote.URL+"/mcp", "yes\n", catalogueModel{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := runtime.Run(ctx, "stock"); done <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("tool request did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("tool request did not cancel")
	}
}
