// Package event defines observable runtime lifecycle events.
package event

import (
	"context"
	"time"
)

// Kind identifies a runtime lifecycle transition.
type Kind string

const (
	// RunStarted is emitted after input validation.
	RunStarted Kind = "run.started"
	// SkillSelected is emitted after routing.
	SkillSelected Kind = "skill.selected"
	// ModelCompleted is emitted after a successful model turn.
	ModelCompleted Kind = "model.completed"
	// ToolStarted is emitted before authorisation and execution.
	ToolStarted Kind = "tool.started"
	// ToolCompleted is emitted after successful tool execution.
	ToolCompleted Kind = "tool.completed"
	// RunCompleted is emitted when the model returns a final answer.
	RunCompleted Kind = "run.completed"
)

// Event is an immutable observation of a runtime transition.
type Event struct {
	Kind       Kind
	OccurredAt time.Time
	RunID      string
	Name       string
	Step       int
}

// Sink receives runtime events. Implementations must be safe for the execution
// model used by their application.
type Sink interface {
	Publish(ctx context.Context, event Event) error
}

// SinkFunc adapts a function to Sink.
type SinkFunc func(ctx context.Context, event Event) error

// Publish delegates delivery to the wrapped function.
func (f SinkFunc) Publish(ctx context.Context, event Event) error {
	return f(ctx, event)
}

// Discard ignores all events.
type Discard struct{}

// Publish always succeeds without retaining the event.
func (Discard) Publish(context.Context, Event) error {
	return nil
}
