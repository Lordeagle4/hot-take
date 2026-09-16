// Package capability resolves vendor-neutral capabilities to concrete tools.
package capability

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	// ErrInvalidName reports an empty or malformed capability name.
	ErrInvalidName = errors.New("invalid capability name")
	// ErrNotFound reports a capability without a registered provider.
	ErrNotFound = errors.New("capability not found")
)

// Registry maps a capability to the names of tools that provide it.
//
// Registry is immutable after construction and safe for concurrent reads.
type Registry struct {
	providers map[string][]string
}

// NewRegistry validates definitions and returns an immutable registry.
func NewRegistry(definitions map[string][]string) (*Registry, error) {
	providers := make(map[string][]string, len(definitions))
	for name, tools := range definitions {
		if err := validateName(name); err != nil {
			return nil, err
		}

		unique := make(map[string]struct{}, len(tools))
		for _, toolName := range tools {
			toolName = strings.TrimSpace(toolName)
			if toolName == "" {
				return nil, fmt.Errorf("%w: capability %q contains an empty tool name", ErrInvalidName, name)
			}
			unique[toolName] = struct{}{}
		}
		if len(unique) == 0 {
			return nil, fmt.Errorf("%w: capability %q has no providers", ErrInvalidName, name)
		}

		providers[name] = make([]string, 0, len(unique))
		for toolName := range unique {
			providers[name] = append(providers[name], toolName)
		}
		sort.Strings(providers[name])
	}

	return &Registry{providers: providers}, nil
}

// Resolve returns the registered tool names for a capability.
func (r *Registry) Resolve(name string) ([]string, error) {
	tools, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
	}

	return append([]string(nil), tools...), nil
}

func validateName(name string) error {
	parts := strings.Split(name, ".")
	if len(parts) < 2 {
		return fmt.Errorf("%w: %q must contain a namespace and action", ErrInvalidName, name)
	}
	for _, part := range parts {
		if part == "" {
			return fmt.Errorf("%w: %q contains an empty segment", ErrInvalidName, name)
		}
		for _, character := range part {
			if character != '_' && character != '-' && (character < 'a' || character > 'z') && (character < '0' || character > '9') {
				return fmt.Errorf("%w: %q contains unsupported characters", ErrInvalidName, name)
			}
		}
	}

	return nil
}
