// Package permission controls whether a runtime may execute requested tools.
package permission

import (
	"context"
	"errors"

	"github.com/Lordeagle4/hot-take/tool"
)

// ErrDenied reports a tool call rejected by the active policy.
var ErrDenied = errors.New("tool call denied")

// Request contains the information available to an authorisation policy.
type Request struct {
	AgentName string
	SkillName string
	Call      tool.Call
}

// Authorizer decides whether the runtime may execute a tool call.
type Authorizer interface {
	Authorize(ctx context.Context, request Request) error
}

// AuthorizeFunc adapts a function to Authorizer.
type AuthorizeFunc func(ctx context.Context, request Request) error

// Authorize delegates the decision to the wrapped function.
func (f AuthorizeFunc) Authorize(ctx context.Context, request Request) error {
	return f(ctx, request)
}

// AllowAll authorises every call. Applications should use it only when the
// registered tools are already constrained to safe, trusted operations.
type AllowAll struct{}

// Authorize always returns nil.
func (AllowAll) Authorize(context.Context, Request) error {
	return nil
}

// DenyAll rejects every call.
type DenyAll struct{}

// Authorize always returns ErrDenied.
func (DenyAll) Authorize(context.Context, Request) error {
	return ErrDenied
}
