// Package agent orchestrates skills, model turns, permissions, and tools.
package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Lordeagle4/hot-take/capability"
	"github.com/Lordeagle4/hot-take/event"
	"github.com/Lordeagle4/hot-take/permission"
	"github.com/Lordeagle4/hot-take/provider"
	"github.com/Lordeagle4/hot-take/skill"
	"github.com/Lordeagle4/hot-take/tool"
)

var (
	// ErrInvalidConfiguration reports a runtime with missing dependencies.
	ErrInvalidConfiguration = errors.New("invalid runtime configuration")
	// ErrEmptyInput reports an empty user message.
	ErrEmptyInput = errors.New("empty input")
	// ErrStepLimit reports a run that did not finish within its configured limit.
	ErrStepLimit = errors.New("agent step limit reached")
)

// Config contains the dependencies and limits required by a Runtime.
type Config struct {
	Name         string
	Instructions string
	MaxSteps     int
	Model        provider.Model
	Skills       *skill.Registry
	Capabilities *capability.Registry
	Tools        *tool.Registry
	Authorizer   permission.Authorizer
	Events       event.Sink
	Clock        func() time.Time
}

// Runtime executes one bounded agent loop per Run call.
type Runtime struct {
	name         string
	instructions string
	maxSteps     int
	model        provider.Model
	skills       *skill.Registry
	capabilities *capability.Registry
	tools        *tool.Registry
	authorizer   permission.Authorizer
	events       event.Sink
	clock        func() time.Time
	sequence     atomic.Uint64
}

// NewRuntime validates config and returns an agent runtime.
func NewRuntime(config Config) (*Runtime, error) {
	if strings.TrimSpace(config.Name) == "" || strings.TrimSpace(config.Instructions) == "" {
		return nil, fmt.Errorf("%w: name and instructions are required", ErrInvalidConfiguration)
	}
	if config.MaxSteps < 1 {
		return nil, fmt.Errorf("%w: MaxSteps must be positive", ErrInvalidConfiguration)
	}
	if config.Model == nil || config.Skills == nil || config.Capabilities == nil || config.Tools == nil || config.Authorizer == nil {
		return nil, fmt.Errorf("%w: model, skills, capabilities, tools, and authorizer are required", ErrInvalidConfiguration)
	}
	if config.Events == nil {
		config.Events = event.Discard{}
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}

	return &Runtime{
		name:         config.Name,
		instructions: config.Instructions,
		maxSteps:     config.MaxSteps,
		model:        config.Model,
		skills:       config.Skills,
		capabilities: config.Capabilities,
		tools:        config.Tools,
		authorizer:   config.Authorizer,
		events:       config.Events,
		clock:        config.Clock,
	}, nil
}

// Run processes one user input until a final answer or error is produced.
func (r *Runtime) Run(ctx context.Context, input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", ErrEmptyInput
	}
	runID := fmt.Sprintf("run-%d", r.sequence.Add(1))
	if err := r.publish(ctx, event.RunStarted, runID, r.name, 0); err != nil {
		return "", err
	}

	selected, err := r.skills.Select(input)
	if err != nil {
		return "", fmt.Errorf("select skill: %w", err)
	}
	if err := r.publish(ctx, event.SkillSelected, runID, selected.Name, 0); err != nil {
		return "", err
	}

	toolNames, err := r.resolveTools(selected.RequiredCapabilities)
	if err != nil {
		return "", fmt.Errorf("resolve skill %q tools: %w", selected.Name, err)
	}
	definitions, err := r.tools.Definitions(toolNames)
	if err != nil {
		return "", fmt.Errorf("describe tools: %w", err)
	}
	items := []provider.Item{{Message: &provider.Message{Role: provider.User, Content: input}}}
	instructions := strings.TrimSpace(r.instructions) + "\n\n" + strings.TrimSpace(selected.Instructions)

	for step := 1; step <= r.maxSteps; step++ {
		turn, generateErr := r.model.Generate(ctx, provider.Request{Instructions: instructions, Items: items, Tools: definitions})
		if generateErr != nil {
			return "", fmt.Errorf("generate model turn %d: %w", step, generateErr)
		}
		if err := r.publish(ctx, event.ModelCompleted, runID, r.name, step); err != nil {
			return "", err
		}
		if len(turn.ToolCalls) == 0 {
			answer := strings.TrimSpace(turn.Text)
			if answer == "" {
				return "", fmt.Errorf("generate model turn %d: provider returned neither text nor tool calls", step)
			}
			if err := r.publish(ctx, event.RunCompleted, runID, r.name, step); err != nil {
				return "", err
			}

			return answer, nil
		}
		if strings.TrimSpace(turn.Text) != "" {
			items = append(items, provider.Item{Message: &provider.Message{Role: provider.Assistant, Content: turn.Text}})
		}

		for _, call := range turn.ToolCalls {
			if strings.TrimSpace(call.ID) == "" || strings.TrimSpace(call.Name) == "" {
				return "", fmt.Errorf("generate model turn %d: tool call ID and name are required", step)
			}
			callCopy := call
			items = append(items, provider.Item{ToolCall: &callCopy})
			if err := r.publish(ctx, event.ToolStarted, runID, call.Name, step); err != nil {
				return "", err
			}
			if err := r.authorizer.Authorize(ctx, permission.Request{AgentName: r.name, SkillName: selected.Name, Call: call}); err != nil {
				return "", fmt.Errorf("authorize tool %q: %w", call.Name, err)
			}
			candidate, err := r.tools.Get(call.Name)
			if err != nil {
				return "", fmt.Errorf("resolve requested tool %q: %w", call.Name, err)
			}
			content, err := candidate.Execute(ctx, call.Arguments)
			if err != nil {
				return "", fmt.Errorf("execute tool %q: %w", call.Name, err)
			}
			result := tool.Result{CallID: call.ID, Content: content}
			items = append(items, provider.Item{ToolResult: &result})
			if err := r.publish(ctx, event.ToolCompleted, runID, call.Name, step); err != nil {
				return "", err
			}
		}
	}

	return "", fmt.Errorf("%w: maximum %d", ErrStepLimit, r.maxSteps)
}

func (r *Runtime) resolveTools(required []string) ([]string, error) {
	names := make([]string, 0, len(required))
	seen := make(map[string]struct{})
	for _, requiredCapability := range required {
		providers, err := r.capabilities.Resolve(requiredCapability)
		if err != nil {
			return nil, err
		}
		for _, name := range providers {
			if _, exists := seen[name]; exists {
				continue
			}
			names = append(names, name)
			seen[name] = struct{}{}
		}
	}

	return names, nil
}

func (r *Runtime) publish(ctx context.Context, kind event.Kind, runID string, name string, step int) error {
	observation := event.Event{Kind: kind, OccurredAt: r.clock(), RunID: runID, Name: name, Step: step}
	if err := r.events.Publish(ctx, observation); err != nil {
		return fmt.Errorf("publish %s event: %w", kind, err)
	}

	return nil
}
