package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Lordeagle4/hot-take/permission"
	"github.com/Lordeagle4/hot-take/project"
	"github.com/Lordeagle4/hot-take/tool"
)

func TestRunDemo(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	if err := run(context.Background(), []string{"demo", "What time is it?"}, strings.NewReader(""), &output); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !strings.Contains(output.String(), "The current UTC time is") {
		t.Fatalf("run() output = %q", output.String())
	}
}

func TestProjectToolsRegistersAllowlistedMCPTool(t *testing.T) {
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
		fmt.Fprintf(writer, `{"jsonrpc":"2.0","id":%d,"result":{"resultType":"complete","tools":[{"name":"repositories.search","description":"Search repositories.","inputSchema":{"type":"object"}},{"name":"admin.delete","description":"Delete data.","inputSchema":{"type":"object"}}]}}`, message.ID)
	}))
	defer server.Close()

	definition := &project.Definition{Plugins: []project.Plugin{{
		Name:      "github",
		Transport: "mcp_streamable_http",
		Endpoint:  server.URL,
		Approval:  "ask",
		Capabilities: []project.PluginCapability{{
			Name:  "repository.search",
			Tools: []string{"repositories.search"},
		}},
	}}}
	tools, capabilities, authorizer, err := projectTools(context.Background(), definition, strings.NewReader("yes\n"), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("projectTools() error = %v", err)
	}
	providers, err := capabilities.Resolve("repository.search")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if len(providers) != 1 || providers[0] != "github__repositories_search" {
		t.Fatalf("Resolve() = %#v", providers)
	}
	if _, err := tools.Get("github__admin_delete"); err == nil {
		t.Fatal("unconfigured server tool was registered")
	}
	if err := authorizer.Authorize(context.Background(), permission.Request{Call: tool.Call{Name: providers[0], Arguments: json.RawMessage(`{}`)}}); err != nil {
		t.Fatalf("Authorize() error = %v", err)
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	t.Parallel()

	err := run(context.Background(), []string{"unknown"}, strings.NewReader(""), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("run() error = %v", err)
	}
}
