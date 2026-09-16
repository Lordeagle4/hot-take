package skill_test

import (
	"testing"

	"github.com/Lordeagle4/hot-take/skill"
)

func TestRegistrySelectsMostSpecificExample(t *testing.T) {
	t.Parallel()

	registry, err := skill.NewRegistry("general", []skill.Skill{
		{Name: "general", Description: "General reasoning", Instructions: "Answer directly."},
		{Name: "clock", Description: "Read time", Instructions: "Use a clock.", Examples: []string{"time", "time in lagos"}},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	selected, err := registry.Select("What is the time in Lagos?")
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if selected.Name != "clock" {
		t.Fatalf("Select() = %q, want clock", selected.Name)
	}
}

func TestRegistryFallsBackToDefault(t *testing.T) {
	t.Parallel()

	registry, err := skill.NewRegistry("general", []skill.Skill{
		{Name: "general", Description: "General reasoning", Instructions: "Answer directly."},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	selected, err := registry.Select("Explain emergence")
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if selected.Name != "general" {
		t.Fatalf("Select() = %q, want general", selected.Name)
	}
}
