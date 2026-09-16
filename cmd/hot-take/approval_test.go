package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Lordeagle4/hot-take/permission"
	"github.com/Lordeagle4/hot-take/project"
	"github.com/Lordeagle4/hot-take/tool"
)

func TestProjectPolicyDefaultsToDeny(t *testing.T) {
	t.Parallel()
	_, _, policy, err := projectTools(context.Background(), &project.Definition{}, strings.NewReader(""), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		denied bool
	}{{"clock_now", false}, {"future_tool", true}} {
		err := policy.Authorize(context.Background(), permission.Request{Call: tool.Call{Name: test.name}})
		if errors.Is(err, permission.ErrDenied) != test.denied || (!test.denied && err != nil) {
			t.Fatalf("%s: %v", test.name, err)
		}
	}
}

func TestTerminalApprovalAnswers(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		input    string
		approved bool
		failure  bool
	}{
		{"yes\n", true, false}, {"Y\n", true, false}, {"\n", false, false}, {"no\n", false, false}, {"yes please\n", false, false}, {"", false, true},
	} {
		approver := newTerminalApprover(strings.NewReader(test.input), &bytes.Buffer{})
		approved, err := approver.Approve(context.Background(), permission.Request{})
		if approved != test.approved || (err != nil) != test.failure {
			t.Fatalf("input %q: approved %v, error %v", test.input, approved, err)
		}
	}
}

func TestTerminalApprovalCancelsBlockedRead(t *testing.T) {
	t.Parallel()
	reader, writer := io.Pipe()
	defer func() {
		if err := reader.Close(); err != nil {
			t.Error(err)
		}
		if err := writer.Close(); err != nil {
			t.Error(err)
		}
	}()
	approver := newTerminalApprover(reader, io.Discard)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	approved, err := approver.Approve(ctx, permission.Request{})
	if approved || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("approved %v, error %v", approved, err)
	}
	// A cancelled approver must never consume another answer for a new request.
	approved, err = approver.Approve(context.Background(), permission.Request{})
	if approved || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("reuse: approved %v, error %v", approved, err)
	}
}

func TestRunRejectsInvalidLimits(t *testing.T) {
	t.Parallel()
	for _, flags := range [][]string{{"-timeout", "0s"}, {"-max-tool-calls", "0"}} {
		args := append([]string{"run"}, flags...)
		args = append(args, "hello")
		err := run(context.Background(), args, strings.NewReader(""), io.Discard)
		if err == nil || !strings.Contains(err.Error(), "must be positive") {
			t.Fatalf("error = %v", err)
		}
	}
}
