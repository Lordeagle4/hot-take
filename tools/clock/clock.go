// Package clock provides a native tool for reading the current time.
package clock

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Lordeagle4/hot-take/tool"
)

// Tool returns the current time from an injected clock.
type Tool struct {
	now func() time.Time
}

// New returns a clock tool. A nil clock uses time.Now.
func New(now func() time.Time) *Tool {
	if now == nil {
		now = time.Now
	}

	return &Tool{now: now}
}

// Definition describes the clock tool and its arguments.
func (*Tool) Definition() tool.Definition {
	return tool.Definition{
		Name:        "clock_now",
		Description: "Return the current time as an RFC 3339 timestamp.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Capabilities: []string{
			"clock.read",
		},
	}
}

// Execute reads the clock. The tool accepts only an empty JSON object.
func (t *Tool) Execute(ctx context.Context, arguments json.RawMessage) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var input map[string]json.RawMessage
	if err := json.Unmarshal(arguments, &input); err != nil {
		return nil, fmt.Errorf("decode arguments: %w", err)
	}
	if len(input) != 0 {
		return nil, fmt.Errorf("clock_now does not accept arguments")
	}

	result, err := json.Marshal(struct {
		Time string `json:"time"`
	}{Time: t.now().UTC().Format(time.RFC3339)})
	if err != nil {
		return nil, fmt.Errorf("encode result: %w", err)
	}

	return result, nil
}
