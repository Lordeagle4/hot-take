// Command hot-take runs the deterministic example agent.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Lordeagle4/hot-take/agent"
	"github.com/Lordeagle4/hot-take/capability"
	"github.com/Lordeagle4/hot-take/permission"
	"github.com/Lordeagle4/hot-take/provider"
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
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, arguments []string) error {
	input := strings.TrimSpace(strings.Join(arguments, " "))
	if input == "" {
		return fmt.Errorf("usage: hot-take <message>")
	}

	tools := tool.NewRegistry()
	if err := tools.Register(clocktool.New(nil)); err != nil {
		return fmt.Errorf("register clock tool: %w", err)
	}
	capabilities, err := capability.NewRegistry(map[string][]string{"clock.read": {"clock_now"}})
	if err != nil {
		return fmt.Errorf("create capability registry: %w", err)
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

	fmt.Println(answer)
	return nil
}
