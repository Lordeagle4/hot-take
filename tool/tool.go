// Package tool defines executable operations exposed to a model.
package tool

import (
	"context"
	"encoding/json"
)

// Definition describes a tool to a model provider.
type Definition struct {
	Name         string
	Description  string
	InputSchema  json.RawMessage
	Capabilities []string
	Strict       bool
}

// Call is a model-requested tool execution.
type Call struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

// Result is the serialisable result of a tool execution.
type Result struct {
	CallID  string
	Content json.RawMessage
}

// Tool exposes one operation to the runtime.
type Tool interface {
	Definition() Definition
	Execute(ctx context.Context, arguments json.RawMessage) (json.RawMessage, error)
}
