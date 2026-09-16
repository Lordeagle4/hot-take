package clock_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	clocktool "github.com/Lordeagle4/hot-take/tools/clock"
)

func TestToolReturnsUTC(t *testing.T) {
	t.Parallel()

	candidate := clocktool.New(func() time.Time {
		return time.Date(2026, 9, 16, 13, 0, 0, 0, time.FixedZone("WAT", 60*60))
	})

	result, err := candidate.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if string(result) != `{"time":"2026-09-16T12:00:00Z"}` {
		t.Fatalf("Execute() = %s", result)
	}
}

func TestToolRejectsArguments(t *testing.T) {
	t.Parallel()

	candidate := clocktool.New(nil)
	if _, err := candidate.Execute(context.Background(), json.RawMessage(`{"timezone":"Africa/Lagos"}`)); err == nil {
		t.Fatal("Execute() error = nil, want argument error")
	}
}
