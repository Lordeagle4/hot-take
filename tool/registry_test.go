package tool_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/Lordeagle4/hot-take/tool"
)

type stubTool struct {
	name string
}

func (s stubTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        s.name,
		Description: "A test tool.",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}
}

func (stubTool) Execute(context.Context, json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

func TestRegistryReturnsSortedUniqueDefinitions(t *testing.T) {
	t.Parallel()

	registry := tool.NewRegistry()
	for _, name := range []string{"zulu", "alpha"} {
		if err := registry.Register(stubTool{name: name}); err != nil {
			t.Fatalf("Register(%q) error = %v", name, err)
		}
	}

	definitions, err := registry.Definitions([]string{"zulu", "alpha", "zulu"})
	if err != nil {
		t.Fatalf("Definitions() error = %v", err)
	}
	got := []string{definitions[0].Name, definitions[1].Name}
	if !reflect.DeepEqual(got, []string{"alpha", "zulu"}) {
		t.Fatalf("Definitions() names = %v", got)
	}
}

func TestRegistryRejectsDuplicateName(t *testing.T) {
	t.Parallel()

	registry := tool.NewRegistry()
	if err := registry.Register(stubTool{name: "duplicate"}); err != nil {
		t.Fatalf("first Register() error = %v", err)
	}
	if err := registry.Register(stubTool{name: "duplicate"}); !errors.Is(err, tool.ErrAlreadyRegistered) {
		t.Fatalf("second Register() error = %v, want ErrAlreadyRegistered", err)
	}
}

func TestRegistryRejectsInvalidSchema(t *testing.T) {
	t.Parallel()

	registry := tool.NewRegistry()
	candidate := invalidSchemaTool{}
	if err := registry.Register(candidate); !errors.Is(err, tool.ErrInvalidDefinition) {
		t.Fatalf("Register() error = %v, want ErrInvalidDefinition", err)
	}
}

type invalidSchemaTool struct{}

func (invalidSchemaTool) Definition() tool.Definition {
	return tool.Definition{Name: "invalid", Description: "Invalid schema.", InputSchema: json.RawMessage(`{`)}
}

func (invalidSchemaTool) Execute(context.Context, json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}
