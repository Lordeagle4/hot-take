package tool

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

var (
	// ErrAlreadyRegistered reports a duplicate tool name.
	ErrAlreadyRegistered = errors.New("tool already registered")
	// ErrNotFound reports an unregistered tool name.
	ErrNotFound = errors.New("tool not found")
	// ErrInvalidDefinition reports malformed tool metadata.
	ErrInvalidDefinition = errors.New("invalid tool definition")
)

// Registry stores tools by their stable names.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry returns an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register validates and stores a tool.
func (r *Registry) Register(candidate Tool) error {
	if candidate == nil {
		return fmt.Errorf("%w: tool is nil", ErrInvalidDefinition)
	}
	definition := candidate.Definition()
	if strings.TrimSpace(definition.Name) == "" || strings.TrimSpace(definition.Description) == "" {
		return fmt.Errorf("%w: name and description are required", ErrInvalidDefinition)
	}
	if len(definition.InputSchema) == 0 {
		return fmt.Errorf("%w: %s has no input schema", ErrInvalidDefinition, definition.Name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[definition.Name]; exists {
		return fmt.Errorf("%w: %s", ErrAlreadyRegistered, definition.Name)
	}
	r.tools[definition.Name] = candidate

	return nil
}

// Get returns a registered tool by name.
func (r *Registry) Get(name string) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	candidate, exists := r.tools[name]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
	}

	return candidate, nil
}

// Definitions returns stable, name-sorted tool metadata.
func (r *Registry) Definitions(names []string) ([]Definition, error) {
	definitions := make([]Definition, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if _, exists := seen[name]; exists {
			continue
		}
		candidate, err := r.Get(name)
		if err != nil {
			return nil, err
		}
		definitions = append(definitions, candidate.Definition())
		seen[name] = struct{}{}
	}
	sort.Slice(definitions, func(i, j int) bool {
		return definitions[i].Name < definitions[j].Name
	})

	return definitions, nil
}
