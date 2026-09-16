package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Lordeagle4/hot-take/permission"
)

// terminalApprover serialises prompts without preventing cancellation while
// waiting for the terminal or for another caller's prompt to finish.
type terminalApprover struct {
	gate    chan struct{}
	reader  *bufio.Reader
	output  io.Writer
	stopped error
}

type approvalLine struct {
	text string
	err  error
}

func newTerminalApprover(input io.Reader, output io.Writer) *terminalApprover {
	return &terminalApprover{gate: make(chan struct{}, 1), reader: bufio.NewReader(input), output: output}
}

func (a *terminalApprover) Approve(ctx context.Context, request permission.Request) (bool, error) {
	select {
	case a.gate <- struct{}{}:
		defer func() { <-a.gate }()
	case <-ctx.Done():
		return false, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.stopped != nil {
		return false, a.stopped
	}
	if _, err := fmt.Fprintf(a.output, "Approve tool %s with arguments %s? [y/N] ", request.Call.Name, request.Call.Arguments); err != nil {
		return false, fmt.Errorf("write approval prompt: %w", err)
	}
	line := make(chan approvalLine, 1)
	go func() {
		answer, err := a.reader.ReadString('\n')
		line <- approvalLine{text: answer, err: err}
	}()
	select {
	case <-ctx.Done():
		// An arbitrary io.Reader cannot be interrupted. Retire this approver so
		// a late answer cannot authorise another request or spawn more reads.
		// The reader's owner must close it to release a blocked read; the CLI
		// exits after cancellation, releasing stdin with the process.
		a.stopped = ctx.Err()
		return false, a.stopped
	case answer := <-line:
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if answer.err != nil && !(errors.Is(answer.err, io.EOF) && answer.text != "") {
			return false, fmt.Errorf("read approval response: %w", answer.err)
		}
		value := strings.ToLower(strings.TrimSpace(answer.text))
		return value == "y" || value == "yes", nil
	}
}
