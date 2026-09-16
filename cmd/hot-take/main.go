// Command hot-take runs agent projects and deterministic local demonstrations.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Lordeagle4/hot-take/agent"
	"github.com/Lordeagle4/hot-take/capability"
	"github.com/Lordeagle4/hot-take/mcp"
	"github.com/Lordeagle4/hot-take/permission"
	"github.com/Lordeagle4/hot-take/project"
	"github.com/Lordeagle4/hot-take/provider"
	openai "github.com/Lordeagle4/hot-take/providers/openai"
	"github.com/Lordeagle4/hot-take/skill"
	"github.com/Lordeagle4/hot-take/tool"
	clocktool "github.com/Lordeagle4/hot-take/tools/clock"
)

const applicationVersion = "0.1.0-dev"

type exampleModel struct{}

func (exampleModel) Generate(_ context.Context, request provider.Request) (provider.Turn, error) {
	for _, item := range request.Items {
		if item.ToolResult != nil {
			var result struct {
				Time string `json:"time"`
			}
			if err := json.Unmarshal(item.ToolResult.Content, &result); err != nil {
				return provider.Turn{}, fmt.Errorf("decode clock result: %w", err)
			}

			return provider.Turn{Text: "The current UTC time is " + result.Time + "."}, nil
		}
	}

	return provider.Turn{ToolCalls: []tool.Call{{ID: "clock-call", Name: "clock_now", Arguments: json.RawMessage(`{}`)}}}, nil
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdin, os.Stdout); err != nil {
		if _, writeErr := fmt.Fprintln(os.Stderr, "error:", err); writeErr != nil {
			os.Exit(1)
		}
		os.Exit(1)
	}
}

func run(ctx context.Context, arguments []string, input io.Reader, output io.Writer) error {
	if len(arguments) == 0 {
		return fmt.Errorf("usage: hot-take <demo|run> [options] <message>")
	}
	switch arguments[0] {
	case "demo":
		return runDemo(ctx, arguments[1:], output)
	case "run":
		return runProject(ctx, arguments[1:], input, output)
	default:
		return fmt.Errorf("unknown command %q: expected demo or run", arguments[0])
	}
}

func runDemo(ctx context.Context, arguments []string, output io.Writer) error {
	input := strings.TrimSpace(strings.Join(arguments, " "))
	if input == "" {
		return fmt.Errorf("usage: hot-take demo <message>")
	}

	tools, capabilities, err := builtInTools()
	if err != nil {
		return err
	}
	skills, err := skill.NewRegistry("clock", []skill.Skill{{
		Name:                 "clock",
		Description:          "Answer questions about the current time.",
		Instructions:         "Use the clock tool before answering.",
		Examples:             []string{"time", "date", "day"},
		RequiredCapabilities: []string{"clock.read"},
	}})
	if err != nil {
		return fmt.Errorf("create skill registry: %w", err)
	}
	runtime, err := agent.NewRuntime(agent.Config{
		Name:         "Base Agent",
		Instructions: "Answer accurately and concisely.",
		MaxSteps:     4,
		Model:        exampleModel{},
		Skills:       skills,
		Capabilities: capabilities,
		Tools:        tools,
		Authorizer:   permission.AllowAll{},
	})
	if err != nil {
		return fmt.Errorf("create runtime: %w", err)
	}
	answer, err := runtime.Run(ctx, input)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(output, answer); err != nil {
		return fmt.Errorf("write answer: %w", err)
	}

	return nil
}

func runProject(ctx context.Context, arguments []string, input io.Reader, output io.Writer) error {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	projectDirectory := flags.String("project", ".", "agent project directory")
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("parse run options: %w", err)
	}
	message := strings.TrimSpace(strings.Join(flags.Args(), " "))
	if message == "" {
		return fmt.Errorf("usage: hot-take run [-project directory] <message>")
	}

	definition, err := (project.Loader{}).Load(*projectDirectory)
	if err != nil {
		return err
	}
	model, err := projectModel(definition.Provider)
	if err != nil {
		return err
	}
	tools, capabilities, authorizer, err := projectTools(ctx, definition, input, output)
	if err != nil {
		return err
	}
	runtime, err := agent.NewRuntime(agent.Config{
		Name:         definition.Name,
		Instructions: definition.Instructions,
		MaxSteps:     definition.MaxSteps,
		Model:        model,
		Skills:       definition.Skills,
		Capabilities: capabilities,
		Tools:        tools,
		Authorizer:   authorizer,
	})
	if err != nil {
		return fmt.Errorf("create runtime: %w", err)
	}
	answer, err := runtime.Run(ctx, message)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(output, answer); err != nil {
		return fmt.Errorf("write answer: %w", err)
	}

	return nil
}

func projectTools(ctx context.Context, definition *project.Definition, input io.Reader, output io.Writer) (*tool.Registry, *capability.Registry, permission.Authorizer, error) {
	tools := tool.NewRegistry()
	if err := tools.Register(clocktool.New(nil)); err != nil {
		return nil, nil, nil, fmt.Errorf("register clock tool: %w", err)
	}
	capabilities := map[string][]string{"clock.read": {"clock_now"}}
	var rules []permission.Rule

	for _, plugin := range definition.Plugins {
		if plugin.Transport != "mcp_streamable_http" {
			return nil, nil, nil, fmt.Errorf("connect plugin %q: unsupported transport %q", plugin.Name, plugin.Transport)
		}
		token := ""
		if plugin.TokenEnvironment != "" {
			var exists bool
			token, exists = os.LookupEnv(plugin.TokenEnvironment)
			if !exists || strings.TrimSpace(token) == "" {
				return nil, nil, nil, fmt.Errorf("connect plugin %q: environment variable %s is required", plugin.Name, plugin.TokenEnvironment)
			}
		}
		client, err := mcp.New(mcp.Config{
			Endpoint:      plugin.Endpoint,
			BearerToken:   token,
			ClientName:    "hot-take",
			ClientVersion: applicationVersion,
			HTTPClient:    &http.Client{Timeout: 30 * time.Second},
		})
		if err != nil {
			return nil, nil, nil, fmt.Errorf("connect plugin %q: %w", plugin.Name, err)
		}
		advertised, err := client.ListTools(ctx)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("discover plugin %q tools: %w", plugin.Name, err)
		}
		definitions := make(map[string]mcp.ToolDefinition, len(advertised))
		for _, candidate := range advertised {
			definitions[candidate.Name] = candidate
		}
		remoteCapabilities := make(map[string][]string)
		for _, mapping := range plugin.Capabilities {
			for _, remoteName := range mapping.Tools {
				if _, exists := definitions[remoteName]; !exists {
					return nil, nil, nil, fmt.Errorf("plugin %q did not advertise configured tool %q", plugin.Name, remoteName)
				}
				remoteCapabilities[remoteName] = append(remoteCapabilities[remoteName], mapping.Name)
			}
		}
		remoteNames := make([]string, 0, len(remoteCapabilities))
		for remoteName := range remoteCapabilities {
			remoteNames = append(remoteNames, remoteName)
		}
		sort.Strings(remoteNames)
		for _, remoteName := range remoteNames {
			bound, err := mcp.NewRemoteTool(client, plugin.Name, definitions[remoteName], remoteCapabilities[remoteName])
			if err != nil {
				return nil, nil, nil, fmt.Errorf("bind plugin %q tool %q: %w", plugin.Name, remoteName, err)
			}
			if err := tools.Register(bound); err != nil {
				return nil, nil, nil, fmt.Errorf("register plugin %q tool %q: %w", plugin.Name, remoteName, err)
			}
			localName := bound.Definition().Name
			for _, capabilityName := range remoteCapabilities[remoteName] {
				capabilities[capabilityName] = append(capabilities[capabilityName], localName)
			}
			rules = append(rules, permission.Rule{Tool: localName, Mode: permission.Mode(plugin.Approval)})
		}
	}

	capabilityRegistry, err := capability.NewRegistry(capabilities)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create capability registry: %w", err)
	}
	policy, err := permission.NewPolicy(permission.Allow, rules, newTerminalApprover(input, output))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create permission policy: %w", err)
	}
	return tools, capabilityRegistry, policy, nil
}

func projectModel(config project.Provider) (provider.Model, error) {
	switch config.Name {
	case "openai":
		model, err := openai.New(openai.Config{APIKey: os.Getenv("OPENAI_API_KEY"), Model: config.Model})
		if err != nil {
			return nil, fmt.Errorf("create OpenAI provider: %w", err)
		}

		return model, nil
	default:
		return nil, fmt.Errorf("unsupported model provider %q", config.Name)
	}
}

func builtInTools() (*tool.Registry, *capability.Registry, error) {
	tools := tool.NewRegistry()
	if err := tools.Register(clocktool.New(nil)); err != nil {
		return nil, nil, fmt.Errorf("register clock tool: %w", err)
	}
	capabilities, err := capability.NewRegistry(map[string][]string{"clock.read": {"clock_now"}})
	if err != nil {
		return nil, nil, fmt.Errorf("create capability registry: %w", err)
	}

	return tools, capabilities, nil
}

type terminalApprover struct {
	mu     sync.Mutex
	reader *bufio.Reader
	output io.Writer
}

func newTerminalApprover(input io.Reader, output io.Writer) *terminalApprover {
	return &terminalApprover{reader: bufio.NewReader(input), output: output}
}

func (a *terminalApprover) Approve(ctx context.Context, request permission.Request) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}
	if _, err := fmt.Fprintf(a.output, "Approve tool %s with arguments %s? [y/N] ", request.Call.Name, request.Call.Arguments); err != nil {
		return false, fmt.Errorf("write approval prompt: %w", err)
	}
	answer, err := a.reader.ReadString('\n')
	if err != nil && !(errors.Is(err, io.EOF) && answer != "") {
		return false, fmt.Errorf("read approval response: %w", err)
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes", nil
}
