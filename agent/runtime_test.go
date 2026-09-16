package agent_test

import (
	"context"
	"encoding/json"
	"errors"
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

type fixedTool struct{}

func (fixedTool) Definition() tool.Definition {
	return tool.Definition{
		Name:         "clock_now",
		Description:  "Return the current time.",
		InputSchema:  json.RawMessage(`{"type":"object","additionalProperties":false}`),
		Capabilities: []string{"clock.read"},
		Strict:       true,
	}
}

func (fixedTool) Execute(context.Context, json.RawMessage) (json.RawMessage, error) {
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

func newRuntime(t *testing.T, model provider.Model, authorizer permission.Authorizer, sink event.Sink) *agent.Runtime {
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

	runtime, err := agent.NewRuntime(agent.Config{
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
	})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}

	return runtime
}
