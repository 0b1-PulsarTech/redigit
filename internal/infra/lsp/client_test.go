package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

// checkCallResponseErr asserts err against code (0 = success, else RPC error).
func checkCallResponseErr(t *testing.T, code int, message string, err error) {
	t.Helper()

	switch code {
	case 0:
		if err != nil {
			t.Fatalf("Call: %v", err)
		}
	case cancelledCode:
		if !errors.Is(err, ErrCancelled) {
			t.Fatalf("err = %v, want ErrCancelled", err)
		}
	default:
		var rpcErr *ResponseError
		if !errors.As(err, &rpcErr) {
			t.Fatalf("err = %v, want a *ResponseError", err)
		}

		if rpcErr.Code != code || rpcErr.Message != message {
			t.Fatalf("got = %+v, want code=%d message=%s", rpcErr, code, message)
		}
	}
}

// TestClient_Call_ServerResponse covers success, an RPC error, and cancelledCode.
func TestClient_Call_ServerResponse(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		code    int
		message string
	}{
		{name: "success"},
		{name: "server error", code: -32000, message: "boom"},
		{name: "cancelled", code: cancelledCode, message: "cancelled"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			client, server, cancel, wait := runningClient(t)
			defer cancel()

			resultCh := make(chan error, 1)

			var result struct {
				OK bool `json:"ok"`
			}

			go func() {
				resultCh <- client.call(context.Background(), "textDocument/references", nil, &result)
			}()

			req := server.next()
			if req.Method != "textDocument/references" {
				t.Fatalf("method = %q, want textDocument/references", req.Method)
			}

			if testCase.code == 0 {
				server.respond(req.ID, map[string]any{"ok": true})
			} else {
				server.respondErr(req.ID, testCase.code, testCase.message)
			}

			err := <-resultCh
			checkCallResponseErr(t, testCase.code, testCase.message, err)

			if testCase.code == 0 && !result.OK {
				t.Fatalf("result.OK = false, want true")
			}

			cancel()

			_ = wait()
		})
	}
}

func TestClient_Call_ContextCancelled(t *testing.T) {
	t.Parallel()

	client, server, runCancel, wait := runningClient(t)
	defer runCancel()

	callCtx, callCancel := context.WithCancel(context.Background())
	resultCh := make(chan error, 1)

	go func() { resultCh <- client.call(callCtx, "textDocument/references", nil, nil) }()

	server.next() // the original request; reading it unblocks its write
	callCancel()

	cancelMsg := server.next()
	if cancelMsg.Method != "$/cancelRequest" {
		t.Fatalf("method = %q, want $/cancelRequest", cancelMsg.Method)
	}

	err := <-resultCh
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}

	runCancel()

	_ = wait()
}

func TestClient_Notify(t *testing.T) {
	t.Parallel()

	client, server := pipePair(t)
	notifyDone := make(chan error, 1)

	go func() { notifyDone <- client.notify(context.Background(), "initialized", struct{}{}) }()

	msg := server.next()

	if err := <-notifyDone; err != nil {
		t.Fatalf("Notify: %v", err)
	}

	if msg.Method != "initialized" {
		t.Fatalf("method = %q, want initialized", msg.Method)
	}

	if len(msg.ID) != 0 {
		t.Fatalf("notification carried an id: %s", msg.ID)
	}
}

func TestClient_Notify_ContextAlreadyCancelled(t *testing.T) {
	t.Parallel()

	client, _ := pipePair(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := client.notify(ctx, "exit", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("notify: err = %v, want context.Canceled", err)
	}
}

func TestClient_Notify_ContextCancelledDuringWrite(t *testing.T) {
	t.Parallel()

	client, server, started := blockedWriterPair(t)
	ctx, cancel := context.WithCancel(context.Background())

	notifyDone := make(chan error, 1)
	go func() { notifyDone <- client.notify(ctx, "exit", nil) }()

	<-started // the write is now blocked inside the pipe, waiting for a reader
	cancel()

	err := <-notifyDone
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("notify: err = %v, want context.Canceled", err)
	}

	// The write cannot be interrupted mid-flight: drain it so its goroutine
	// does not stay blocked past the test.
	server.next()
}

// TestClient_Call_UnblocksDespiteAbandonedNotifyWrite reproduces round 2:
// an abandoned notify write holds writeMu, but call races its own ctx.
func TestClient_Call_UnblocksDespiteAbandonedNotifyWrite(t *testing.T) {
	t.Parallel()

	rpcClient, _, started := blockedWriterPair(t)
	notifyCtx, notifyCancel := context.WithCancel(context.Background())

	notifyDone := make(chan error, 1)
	go func() { notifyDone <- rpcClient.notify(notifyCtx, "exit", nil) }()

	<-started // the notify write is now blocked inside the pipe, no reader drains it
	notifyCancel()

	if err := <-notifyDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("notify: err = %v, want context.Canceled", err)
	}

	// The abandoned write is still running, holding writeMu; call's own
	// short-deadline ctx must give up instead of waiting behind writeMu.
	callCtx, callCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer callCancel()

	callDone := make(chan error, 1)
	go func() { callDone <- rpcClient.call(callCtx, "textDocument/references", nil, nil) }()

	if err := <-callDone; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("call: err = %v, want context.DeadlineExceeded", err)
	}
}

func TestClient_Cancel_SendsDespiteExpiredContext(t *testing.T) {
	t.Parallel()

	client, server := pipePair(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // ctx is already done, as it always is by the time Call reaches cancel

	// cancel's own write blocks on the pipe until read, so it must run
	// concurrently with server.next() below rather than before it.
	waitCancel := runAsync(func() { client.cancel(ctx, json.RawMessage("42")) })

	msg := server.next()

	waitCancel()

	if msg.Method != "$/cancelRequest" {
		t.Fatalf("method = %q, want $/cancelRequest", msg.Method)
	}

	var params cancelParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}

	if string(params.ID) != "42" {
		t.Fatalf("cancelParams.ID = %s, want 42", params.ID)
	}
}

func TestClient_ConcurrentCalls(t *testing.T) {
	t.Parallel()

	client, server, cancel, wait := runningClient(t)
	defer cancel()

	const callers = 8

	callCtx := context.Background()
	start := make(chan struct{})
	errs := make([]error, callers)

	var wg sync.WaitGroup

	for i := range callers {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()
			<-start

			var result struct {
				N int `json:"n"`
			}

			errs[i] = client.call(callCtx, "test/echo", map[string]any{"n": i}, &result)
			if errs[i] == nil && result.N != i {
				errs[i] = errors.New("echoed wrong value")
			}
		}(i)
	}

	close(start)

	for range callers {
		req := server.next()

		var params struct {
			N int `json:"n"`
		}

		if err := json.Unmarshal(req.Params, &params); err != nil {
			t.Fatalf("unmarshal params: %v", err)
		}

		server.respond(req.ID, map[string]any{"n": params.N})
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("caller %d: %v", i, err)
		}
	}

	cancel()

	_ = wait()
}

// TestClient_Call_ContextAlreadyExpired_SkipsBackgroundCancel is NIT5.
func TestClient_Call_ContextAlreadyExpired_SkipsBackgroundCancel(t *testing.T) {
	t.Parallel()

	rpcClient, _ := pipePair(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := rpcClient.call(ctx, "textDocument/references", nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("call: err = %v, want context.Canceled", err)
	}

	if !waitBounded(50*time.Millisecond, rpcClient.waitBackground) {
		t.Fatalf("waitBackground: blocked; a cancel goroutine ran despite no send being attempted")
	}
}

// TestClient_Call_JoinsBackgroundCancelOnExpiredWrite: the spawned goroutine never stays dangling.
func TestClient_Call_JoinsBackgroundCancelOnExpiredWrite(t *testing.T) {
	t.Parallel()

	rpcClient, server, started := blockedWriterPair(t)
	ctx, cancel := context.WithCancel(context.Background())
	callDone := make(chan error, 1)

	go func() { callDone <- rpcClient.call(ctx, "textDocument/references", nil, nil) }()

	<-started // the initial send is now blocked inside the pipe, in flight
	cancel()  // ctx fires while the write was already attempted

	if err := <-callDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("call: err = %v, want context.Canceled", err)
	}

	server.next() // drains the original request, freeing the first write

	// The background cancel's own write is now blocked on the pipe (nothing
	// has read it yet): a real WaitGroup must not let waitBackground return
	// before it does - an untracked fire-and-forget goroutine would return
	// here immediately instead, which is exactly the bug this proves fixed.
	backgroundDone := make(chan struct{})
	go func() {
		defer close(backgroundDone)
		rpcClient.waitBackground()
	}()

	select {
	case <-backgroundDone:
		t.Fatalf("waitBackground: returned before the background cancel's write was drained")
	case <-time.After(20 * time.Millisecond):
	}

	server.next() // drains the background $/cancelRequest, freeing its write

	if !waitBounded(time.Second, func() { <-backgroundDone }) {
		t.Fatalf("waitBackground: still blocked after the background cancel finished sending")
	}
}
