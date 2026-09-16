package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/Lordeagle4/hot-take/agent"
	"github.com/Lordeagle4/hot-take/capability"
	"github.com/Lordeagle4/hot-take/event"
	"github.com/Lordeagle4/hot-take/permission"
	"github.com/Lordeagle4/hot-take/provider"
	"github.com/Lordeagle4/hot-take/skill"
	"github.com/Lordeagle4/hot-take/tool"
)

type scriptedModel struct {
	turns    []provider.Turn
	requests []provider.Request
}

func (m *scriptedModel) Generate(_ context.Context, request provider.Request) (provider.Turn, error) {
	m.requests = append(m.requests, request)
	if len(m.turns) == 0 {
		return provider.Turn{}, errors.New("script exhausted")
	}
	turn := m.turns[0]
	m.turns = m.turns[1:]

	return turn, nil
}

type fixedTool struct{ executions *int }

func (fixedTool) Definition() tool.Definition {
	return tool.Definition{
		Name:         "clock_now",
		Description:  "Return the current time.",
		InputSchema:  json.RawMessage(`{"type":"object","additionalProperties":false}`),
		Capabilities: []string{"clock.read"},
		Strict:       true,
	}
}

func (f fixedTool) Execute(context.Context, json.RawMessage) (json.RawMessage, error) {
	if f.executions != nil {
		*f.executions++
	}
	return json.RawMessage(`{"time":"2026-09-16T12:00:00Z"}`), nil
}

func TestRuntimeExecutesAuthorisedToolLoop(t *testing.T) {
	t.Parallel()

	model := &scriptedModel{turns: []provider.Turn{
		{ToolCalls: []tool.Call{{ID: "call-1", Name: "clock_now", Arguments: json.RawMessage(`{}`)}}, State: json.RawMessage(`{"cursor":"next"}`)},
		{Text: "It is noon UTC."},
	}}
	events := make([]event.Event, 0)
	runtime := newRuntime(t, model, permission.AllowAll{}, event.SinkFunc(func(_ context.Context, observation event.Event) error {
		events = append(events, observation)
		return nil
	}))

	answer, err := runtime.Run(context.Background(), "What time is it?")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "It is noon UTC." {
		t.Fatalf("Run() = %q", answer)
	}
	if len(model.requests) != 2 {
		t.Fatalf("Generate() calls = %d, want 2", len(model.requests))
	}
	if string(model.requests[1].State) != `{"cursor":"next"}` {
		t.Fatalf("second request state = %s", model.requests[1].State)
	}
	secondItems := model.requests[1].Items
	if len(secondItems) != 3 || secondItems[1].ToolCall == nil {
		t.Fatalf("second request items = %#v, want user message, tool call, and tool result", secondItems)
	}
	result := secondItems[2].ToolResult
	if result == nil || string(result.Content) != `{"time":"2026-09-16T12:00:00Z"}` {
		t.Fatalf("second request tool result = %#v", result)
	}

	wantKinds := []event.Kind{event.RunStarted, event.SkillSelected, event.ModelCompleted, event.ToolStarted, event.ToolCompleted, event.ModelCompleted, event.RunCompleted}
	gotKinds := make([]event.Kind, 0, len(events))
	for _, observation := range events {
		gotKinds = append(gotKinds, observation.Kind)
	}
	if !reflect.DeepEqual(gotKinds, wantKinds) {
		t.Fatalf("event kinds = %v, want %v", gotKinds, wantKinds)
	}
}

func TestRuntimeStopsDeniedTool(t *testing.T) {
	t.Parallel()

	model := &scriptedModel{turns: []provider.Turn{{ToolCalls: []tool.Call{{ID: "call-1", Name: "clock_now", Arguments: json.RawMessage(`{}`)}}}}}
	runtime := newRuntime(t, model, permission.DenyAll{}, event.Discard{})

	_, err := runtime.Run(context.Background(), "What time is it?")
	if !errors.Is(err, permission.ErrDenied) {
		t.Fatalf("Run() error = %v, want ErrDenied", err)
	}
}

func TestRuntimeEnforcesStepLimit(t *testing.T) {
	t.Parallel()

	model := &scriptedModel{turns: []provider.Turn{
		{ToolCalls: []tool.Call{{ID: "call-1", Name: "clock_now", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []tool.Call{{ID: "call-2", Name: "clock_now", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []tool.Call{{ID: "call-3", Name: "clock_now", Arguments: json.RawMessage(`{}`)}}},
	}}
	runtime := newRuntime(t, model, permission.AllowAll{}, event.Discard{})

	_, err := runtime.Run(context.Background(), "What time is it?")
	if !errors.Is(err, agent.ErrStepLimit) {
		t.Fatalf("Run() error = %v, want ErrStepLimit", err)
	}
}

func newRuntime(t *testing.T, model provider.Model, authorizer permission.Authorizer, sink event.Sink, options ...func(*agent.Config)) *agent.Runtime {
	t.Helper()

	tools := tool.NewRegistry()
	if err := tools.Register(fixedTool{}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	capabilities, err := capability.NewRegistry(map[string][]string{"clock.read": {"clock_now"}})
	if err != nil {
		t.Fatalf("NewRegistry(capability) error = %v", err)
	}
	skills, err := skill.NewRegistry("general", []skill.Skill{
		{Name: "general", Description: "General reasoning", Instructions: "Answer directly."},
		{Name: "clock", Description: "Time questions", Instructions: "Read the clock before answering.", Examples: []string{"time"}, RequiredCapabilities: []string{"clock.read"}},
	})
	if err != nil {
		t.Fatalf("NewRegistry(skill) error = %v", err)
	}

	config := agent.Config{
		Name:         "Test agent",
		Instructions: "Be accurate.",
		MaxSteps:     3,
		Model:        model,
		Skills:       skills,
		Capabilities: capabilities,
		Tools:        tools,
		Authorizer:   authorizer,
		Events:       sink,
		Clock:        func() time.Time { return time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC) },
	}
	for _, option := range options {
		option(&config)
	}
	runtime, err := agent.NewRuntime(config)
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}

	return runtime
}

func TestRuntimeRejectsRegisteredToolOutsideSkill(t *testing.T) {
	t.Parallel()
	model := &scriptedModel{turns: []provider.Turn{{ToolCalls: []tool.Call{{ID: "call", Name: "clock_now", Arguments: json.RawMessage(`{}`)}}}}}
	authorised := false
	runtime := newRuntime(t, model, permission.AuthorizeFunc(func(context.Context, permission.Request) error { authorised = true; return nil }), event.Discard{})
	_, err := runtime.Run(context.Background(), "Hello")
	if !errors.Is(err, agent.ErrToolNotSelected) || authorised {
		t.Fatalf("error = %v, authorised = %v", err, authorised)
	}
}

func TestRuntimeToolBudget(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name               string
		batches            []int
		wantAuthorisations int
	}{
		{"oversized first batch", []int{3}, 0},
		{"cumulative budget", []int{1, 2}, 1},
		{"exact budget", []int{2}, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := &scriptedModel{}
			for _, count := range test.batches {
				turn := provider.Turn{}
				for index := 0; index < count; index++ {
					turn.ToolCalls = append(turn.ToolCalls, tool.Call{ID: fmt.Sprintf("call-%d-%d", len(model.turns), index), Name: "clock_now", Arguments: json.RawMessage(`{}`)})
				}
				model.turns = append(model.turns, turn)
			}
			model.turns = append(model.turns, provider.Turn{Text: "Done"})
			authorised := 0
			runtime := newRuntime(t, model, permission.AuthorizeFunc(func(context.Context, permission.Request) error { authorised++; return nil }), event.Discard{}, func(config *agent.Config) { config.MaxToolCalls = 2 })
			_, err := runtime.Run(context.Background(), "time")
			if test.name == "exact budget" {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, agent.ErrToolCallLimit) {
				t.Fatalf("error = %v", err)
			}
			if authorised != test.wantAuthorisations {
				t.Fatalf("authorisations = %d", authorised)
			}
		})
	}
}

type waitingModel struct{}

func (waitingModel) Generate(ctx context.Context, _ provider.Request) (provider.Turn, error) {
	<-ctx.Done()
	return provider.Turn{}, ctx.Err()
}

func TestRuntimeDeadline(t *testing.T) {
	t.Parallel()
	runtime := newRuntime(t, waitingModel{}, permission.AllowAll{}, event.Discard{}, func(config *agent.Config) { config.RunTimeout = 10 * time.Millisecond })
	_, err := runtime.Run(context.Background(), "time")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v", err)
	}
}

func TestRuntimeCancelledBeforeModel(t *testing.T) {
	t.Parallel()
	model := &scriptedModel{}
	runtime := newRuntime(t, model, permission.AllowAll{}, event.Discard{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := runtime.Run(ctx, "time")
	if !errors.Is(err, context.Canceled) || len(model.requests) != 0 {
		t.Fatalf("error = %v, requests = %d", err, len(model.requests))
	}
}

func TestRuntimeCancellationAfterApproval(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	model := &scriptedModel{turns: []provider.Turn{{ToolCalls: []tool.Call{{ID: "call", Name: "clock_now", Arguments: json.RawMessage(`{}`)}}}}}
	executions := 0
	runtime := newRuntime(t, model, permission.AuthorizeFunc(func(context.Context, permission.Request) error { cancel(); return nil }), event.Discard{}, func(config *agent.Config) {
		registry := tool.NewRegistry()
		if err := registry.Register(fixedTool{executions: &executions}); err != nil {
			t.Fatal(err)
		}
		config.Tools = registry
	})
	_, err := runtime.Run(ctx, "time")
	if !errors.Is(err, context.Canceled) || executions != 0 {
		t.Fatalf("error = %v, executions = %d", err, executions)
	}
}
