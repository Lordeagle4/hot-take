package capability_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Lordeagle4/hot-take/capability"
)

func TestRegistryResolveReturnsSortedCopy(t *testing.T) {
	t.Parallel()

	registry, err := capability.NewRegistry(map[string][]string{
		"clock.read": {"world_time", "local_time", "world_time"},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	got, err := registry.Resolve("clock.read")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	want := []string{"local_time", "world_time"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Resolve() = %v, want %v", got, want)
	}

	got[0] = "mutated"
	again, err := registry.Resolve("clock.read")
	if err != nil {
		t.Fatalf("Resolve() second call error = %v", err)
	}
	if !reflect.DeepEqual(again, want) {
		t.Fatalf("Resolve() exposed internal state: %v", again)
	}
}

func TestRegistryRejectsInvalidCapability(t *testing.T) {
	t.Parallel()

	_, err := capability.NewRegistry(map[string][]string{"clock": {"local_time"}})
	if !errors.Is(err, capability.ErrInvalidName) {
		t.Fatalf("NewRegistry() error = %v, want ErrInvalidName", err)
	}
}

func TestRegistryReportsMissingCapability(t *testing.T) {
	t.Parallel()

	registry, err := capability.NewRegistry(map[string][]string{"clock.read": {"local_time"}})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	_, err = registry.Resolve("weather.read")
	if !errors.Is(err, capability.ErrNotFound) {
		t.Fatalf("Resolve() error = %v, want ErrNotFound", err)
	}
}
