// Package permission controls whether a runtime may execute requested tools.
package permission

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Lordeagle4/hot-take/tool"
)

// ErrDenied reports a tool call rejected by the active policy.
var ErrDenied = errors.New("tool call denied")

// Mode defines how a Policy handles a matching tool call.
type Mode string

const (
	// Allow permits a tool call without interaction.
	Allow Mode = "allow"
	// Deny rejects a tool call.
	Deny Mode = "deny"
	// Ask delegates the decision to an Approver.
	Ask Mode = "ask"
)

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

// Approver obtains a human or application decision for one tool call.
type Approver interface {
	Approve(ctx context.Context, request Request) (bool, error)
}

// ApproveFunc adapts a function to Approver.
type ApproveFunc func(ctx context.Context, request Request) (bool, error)

// Approve delegates the decision to the wrapped function.
func (f ApproveFunc) Approve(ctx context.Context, request Request) (bool, error) {
	return f(ctx, request)
}

// Rule assigns a permission mode to one exact tool name.
type Rule struct {
	Tool string
	Mode Mode
}

// Policy applies exact-name rules and a default mode to tool calls.
//
// Policy is immutable after construction and safe for concurrent use when its
// Approver is safe for concurrent use.
type Policy struct {
	defaultMode Mode
	rules       map[string]Mode
	approver    Approver
}

// NewPolicy validates rules and returns an immutable permission policy.
func NewPolicy(defaultMode Mode, rules []Rule, approver Approver) (*Policy, error) {
	if !validMode(defaultMode) {
		return nil, fmt.Errorf("invalid default permission mode %q", defaultMode)
	}
	validated := make(map[string]Mode, len(rules))
	requiresApproval := defaultMode == Ask
	for _, rule := range rules {
		name := strings.TrimSpace(rule.Tool)
		if name == "" {
			return nil, fmt.Errorf("permission rule tool name is required")
		}
		if !validMode(rule.Mode) {
			return nil, fmt.Errorf("invalid permission mode %q for tool %q", rule.Mode, name)
		}
		if _, exists := validated[name]; exists {
			return nil, fmt.Errorf("duplicate permission rule for tool %q", name)
		}
		validated[name] = rule.Mode
		requiresApproval = requiresApproval || rule.Mode == Ask
	}
	if requiresApproval && approver == nil {
		return nil, fmt.Errorf("an approver is required by the permission policy")
	}

	return &Policy{defaultMode: defaultMode, rules: validated, approver: approver}, nil
}

// Authorize applies the configured rule for the requested tool.
func (p *Policy) Authorize(ctx context.Context, request Request) error {
	mode, exists := p.rules[request.Call.Name]
	if !exists {
		mode = p.defaultMode
	}
	switch mode {
	case Allow:
		return nil
	case Deny:
		return fmt.Errorf("%w: %s", ErrDenied, request.Call.Name)
	case Ask:
		approved, err := p.approver.Approve(ctx, request)
		if err != nil {
			return fmt.Errorf("approve tool %q: %w", request.Call.Name, err)
		}
		if !approved {
			return fmt.Errorf("%w: %s", ErrDenied, request.Call.Name)
		}
		return nil
	default:
		return fmt.Errorf("invalid permission mode %q", mode)
	}
}

func validMode(mode Mode) bool {
	return mode == Allow || mode == Deny || mode == Ask
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
