package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunDemo(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	if err := run(context.Background(), []string{"demo", "What time is it?"}, &output); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !strings.Contains(output.String(), "The current UTC time is") {
		t.Fatalf("run() output = %q", output.String())
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	t.Parallel()

	err := run(context.Background(), []string{"unknown"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("run() error = %v", err)
	}
}
