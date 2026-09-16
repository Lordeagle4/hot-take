// Package skill defines reusable task instructions and deterministic routing.
package skill

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	// ErrInvalid reports a malformed skill definition.
	ErrInvalid = errors.New("invalid skill")
	// ErrNoMatch reports that no skill can handle an input.
	ErrNoMatch = errors.New("no matching skill")
)

// Skill contains task-specific judgement without executable behaviour.
type Skill struct {
	Name                 string
	Description          string
	Instructions         string
	Examples             []string
	RequiredCapabilities []string
}

// Registry stores validated skills and selects the best match for an input.
type Registry struct {
	defaultName string
	skills      map[string]Skill
}

// NewRegistry validates skills and returns an immutable registry.
func NewRegistry(defaultName string, skills []Skill) (*Registry, error) {
	registered := make(map[string]Skill, len(skills))
	for _, candidate := range skills {
		if err := validate(candidate); err != nil {
			return nil, err
		}
		if _, exists := registered[candidate.Name]; exists {
			return nil, fmt.Errorf("%w: duplicate name %q", ErrInvalid, candidate.Name)
		}
		candidate.Examples = append([]string(nil), candidate.Examples...)
		candidate.RequiredCapabilities = append([]string(nil), candidate.RequiredCapabilities...)
		registered[candidate.Name] = candidate
	}
	if _, exists := registered[defaultName]; !exists {
		return nil, fmt.Errorf("%w: default skill %q is not registered", ErrInvalid, defaultName)
	}

	return &Registry{defaultName: defaultName, skills: registered}, nil
}

// Select returns the most relevant skill using documented example phrases.
// Ties are resolved by skill name to keep routing deterministic.
func (r *Registry) Select(input string) (Skill, error) {
	normalised := strings.ToLower(strings.TrimSpace(input))
	if normalised == "" {
		return Skill{}, fmt.Errorf("%w: input is empty", ErrNoMatch)
	}

	type scoredSkill struct {
		skill Skill
		score int
	}
	scored := make([]scoredSkill, 0, len(r.skills))
	for _, candidate := range r.skills {
		score := 0
		for _, example := range candidate.Examples {
			if strings.Contains(normalised, strings.ToLower(example)) {
				score += len(example)
			}
		}
		scored = append(scored, scoredSkill{skill: candidate, score: score})
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].skill.Name < scored[j].skill.Name
		}
		return scored[i].score > scored[j].score
	})
	if len(scored) > 0 && scored[0].score > 0 {
		return scored[0].skill, nil
	}

	return r.skills[r.defaultName], nil
}

func validate(candidate Skill) error {
	if strings.TrimSpace(candidate.Name) == "" || strings.TrimSpace(candidate.Description) == "" || strings.TrimSpace(candidate.Instructions) == "" {
		return fmt.Errorf("%w: name, description, and instructions are required", ErrInvalid)
	}
	for _, capability := range candidate.RequiredCapabilities {
		if strings.TrimSpace(capability) == "" {
			return fmt.Errorf("%w: skill %q has an empty capability", ErrInvalid, candidate.Name)
		}
	}

	return nil
}
