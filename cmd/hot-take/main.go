// Command hot-take runs agent projects and deterministic local demonstrations.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Lordeagle4/hot-take/agent"
	"github.com/Lordeagle4/hot-take/capability"
	"github.com/Lordeagle4/hot-take/permission"
	"github.com/Lordeagle4/hot-take/project"
	"github.com/Lordeagle4/hot-take/provider"
	openai "github.com/Lordeagle4/hot-take/providers/openai"
	"github.com/Lordeagle4/hot-take/skill"
	"github.com/Lordeagle4/hot-take/tool"
	clocktool "github.com/Lordeagle4/hot-take/tools/clock"
)

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
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		if _, writeErr := fmt.Fprintln(os.Stderr, "error:", err); writeErr != nil {
			os.Exit(1)
		}
		os.Exit(1)
	}
}

func run(ctx context.Context, arguments []string, output io.Writer) error {
	if len(arguments) == 0 {
		return fmt.Errorf("usage: hot-take <demo|run> [options] <message>")
	}
	switch arguments[0] {
	case "demo":
		return runDemo(ctx, arguments[1:], output)
	case "run":
		return runProject(ctx, arguments[1:], output)
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

func runProject(ctx context.Context, arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	projectDirectory := flags.String("project", ".", "agent project directory")
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("parse run options: %w", err)
	}
	input := strings.TrimSpace(strings.Join(flags.Args(), " "))
	if input == "" {
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
	tools, capabilities, err := builtInTools()
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
