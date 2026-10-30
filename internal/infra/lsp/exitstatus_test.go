package lsp

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"testing"
)

func TestSession_CloseAfterAHandshakeThatSucceeded(t *testing.T) {
	t.Parallel()

	errWait := errors.New("wait failed")

	testCases := []struct {
		name    string
		exit    error
		wantErr error
	}{
		{"a process that exited", nil, nil},
		{"a process that exited by signal", &exec.ExitError{}, nil},
		{"a wrapped exit by signal", fmt.Errorf("wait: %w", &exec.ExitError{}), nil},
		{"a wait that failed for another reason", errWait, errWait},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			clientWriter, clientReader, server := pipeTransport(t)
			processCtx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)

			wait := func() error { return testCase.exit }
			kill := func() error { return nil }
			session := newSession(
				processCtx, cancel, clientWriter, clientReader, testLogger(), wait, kill,
			)

			closeDone := make(chan error, 1)
			go func() { closeDone <- session.Close(context.Background()) }()

			server.respond(server.next().ID, nil)

			if exitNotify := server.next(); exitNotify.Method != "exit" {
				t.Fatalf("method = %q, want exit", exitNotify.Method)
			}

			if err := <-closeDone; !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Close() error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}
