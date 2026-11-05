package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// TestClient_Run_ClosesWriterOnContextDone: run's watcher force-closes c.w
// once its OWN ctx is done, freeing a wedged write's writeMu.
func TestClient_Run_ClosesWriterOnContextDone(t *testing.T) {
	t.Parallel()

	rpcClient, _, started := blockedWriterPair(t)
	runCtx, runCancel := context.WithCancel(context.Background())
	wait := startRun(t, runCtx, rpcClient)

	notifyDone := make(chan error, 1)
	go func() { notifyDone <- rpcClient.notify(context.Background(), "exit", nil) }()

	<-started // the write is now blocked inside the pipe, waiting for a reader
	runCancel()

	err := <-notifyDone
	if err == nil {
		t.Fatalf("notify: want a write error once run's ctx force-closes the writer, got nil")
	}

	if errors.Is(err, context.Canceled) {
		t.Fatalf("notify: err = %v, want a write error, not ctx cancellation", err)
	}

	if err := wait(); !errors.Is(err, context.Canceled) {
		t.Fatalf("run: err = %v, want context.Canceled", err)
	}
}

func TestClient_AnswersServerRequest(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		method     string
		params     any
		wantResult string // "null" or "array"
	}{
		{name: "register capability", method: "client/registerCapability", wantResult: "null"},
		{name: "unregister capability", method: "client/unregisterCapability", wantResult: "null"},
		{
			name:       "work done progress create",
			method:     "window/workDoneProgress/create",
			wantResult: "null",
		},
		{
			name:       "workspace configuration",
			method:     "workspace/configuration",
			params:     map[string]any{"items": []map[string]any{{}, {}}},
			wantResult: "array",
		},
		{name: "unknown method rejected", method: "totally/unknown", wantResult: "error"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, server, cancel, wait := runningClient(t)
			defer cancel()

			server.sendRequest(json.RawMessage("99"), testCase.method, testCase.params)

			resp := server.next()

			switch testCase.wantResult {
			case "null":
				if string(resp.Result) != "null" {
					t.Fatalf("result = %s, want null", resp.Result)
				}
			case "array":
				var arr []any
				if err := json.Unmarshal(resp.Result, &arr); err != nil {
					t.Fatalf("unmarshal result: %v", err)
				}

				if len(arr) != 2 {
					t.Fatalf("len(result) = %d, want 2", len(arr))
				}
			case "error":
				if resp.Error == nil || resp.Error.Code != methodNotFoundCode {
					t.Fatalf("error = %+v, want method-not-found", resp.Error)
				}
			}

			cancel()

			_ = wait()
		})
	}
}

func TestClient_TransportClosed(t *testing.T) {
	t.Parallel()

	client, server := pipePair(t)
	wait := startRun(t, t.Context(), client)

	resultCh := make(chan error, 1)
	go func() { resultCh <- client.call(context.Background(), "textDocument/references", nil, nil) }()

	server.next()
	server.hangup()

	if err := <-resultCh; err == nil {
		t.Fatalf("Call: want error once the transport closed, got nil")
	}

	runErr := wait()
	if runErr == nil {
		t.Fatalf("run: want an error once the transport closed, got nil")
	}
}

func TestClient_Run_StopsOnContextDone(t *testing.T) {
	t.Parallel()

	_, _, cancel, wait := runningClient(t)

	cancel()

	if err := wait(); !errors.Is(err, context.Canceled) {
		t.Fatalf("run: err = %v, want context.Canceled", err)
	}
}
