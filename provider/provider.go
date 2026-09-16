// Package provider defines the model boundary used by the agent runtime.
package provider

import (
	"context"

	"github.com/Lordeagle4/hot-take/tool"
)

// Role identifies the author of a transcript message.
type Role string

const (
	// User identifies human input.
	User Role = "user"
	// Assistant identifies model output.
	Assistant Role = "assistant"
)

// Message is a textual transcript item.
type Message struct {
	Role    Role
	Content string
}

// Item is one model input item. Exactly one field must be set.
type Item struct {
	Message    *Message
	ToolCall   *tool.Call
	ToolResult *tool.Result
}

// Request contains the complete context for one model turn.
type Request struct {
	Instructions string
	Items        []Item
	Tools        []tool.Definition
}

// Turn is one provider response. A final turn has text and no tool calls.
type Turn struct {
	Text      string
	ToolCalls []tool.Call
}

// Model produces the next agent turn.
type Model interface {
	Generate(ctx context.Context, request Request) (Turn, error)
}
