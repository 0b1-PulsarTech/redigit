package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/wrapped-owls/redigit/internal/transport/cli"
)

func TestRun(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("boom")

	testCases := []struct {
		name     string
		execute  runner
		wantCode int
	}{
		{
			name: "success returns exit 0",
			execute: func(context.Context, []string, io.Writer, io.Writer) error {
				return nil
			},
			wantCode: 0,
		},
		{
			name: "a usage error returns exit 2",
			execute: func(context.Context, []string, io.Writer, io.Writer) error {
				return cli.ErrUsage
			},
			wantCode: exitUsage,
		},
		{
			name: "a wrapped usage error still returns exit 2",
			execute: func(context.Context, []string, io.Writer, io.Writer) error {
				return errors.Join(cli.ErrUsage, errBoom)
			},
			wantCode: exitUsage,
		},
		{
			name: "any other failure returns exit 1",
			execute: func(context.Context, []string, io.Writer, io.Writer) error {
				return errBoom
			},
			wantCode: 1,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer

			got := run(context.Background(), nil, &stdout, &stderr, testCase.execute)
			if got != testCase.wantCode {
				t.Fatalf("run() = %d, want %d", got, testCase.wantCode)
			}
		})
	}
}

// TestRunLogsTheFailureOnce pins that run does not also return the error for
// main to log a second time: main is the top of the chain, so logging here is
// the one correct place, and a second log site would double-report every run.
func TestRunLogsTheFailureOnce(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	code := run(
		context.Background(), nil, &stdout, &stderr,
		func(context.Context, []string, io.Writer, io.Writer) error {
			return errors.New("disk on fire")
		},
	)

	if code != 1 {
		t.Fatalf("run() = %d, want 1", code)
	}

	if stderr.Len() == 0 {
		t.Fatal("expected the failure to be logged to stderr")
	}
}
