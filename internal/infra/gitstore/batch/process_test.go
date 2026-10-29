//go:build unix

package batch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func isReaped(pid int) bool { return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) }

func TestStreamReadsLargeBlob(t *testing.T) {
	t.Parallel()

	repo := newRepo(t)
	content := strings.Repeat("0123456789abcdef", 1<<16)
	commitFiles(t, repo, "big", map[string]string{"big.bin": content, "small.go": "x\n"})

	stream := NewStream(repo)

	t.Cleanup(func() { _ = stream.Close() })

	contents := map[string]string{"HEAD:big.bin": content, "HEAD:small.go": "x\n"}

	for _, spec := range []string{"HEAD:big.bin", "HEAD:small.go", "HEAD:big.bin"} {
		got, err := stream.Read(t.Context(), spec)
		if err != nil {
			t.Fatalf("Read(%s) err = %v", spec, err)
		}

		if string(got) != contents[spec] {
			t.Fatalf("Read(%s) returned %d bytes, want %d", spec, len(got), len(contents[spec]))
		}
	}
}

func TestStreamCloseReapsTheChild(t *testing.T) {
	t.Parallel()

	repo := newRepo(t)
	commitFiles(t, repo, "one", map[string]string{"a.go": "alpha\n"})

	stream := NewStream(repo)

	if _, err := stream.Read(t.Context(), "HEAD:a.go"); err != nil {
		t.Fatalf("warm-up read: %v", err)
	}

	pid := stream.proc.cmd.Process.Pid

	for attempt := range 2 {
		if err := stream.Close(); err != nil {
			t.Fatalf("Close() #%d err = %v", attempt+1, err)
		}
	}

	if !isReaped(pid) {
		t.Fatal("cat-file was not reaped by Close")
	}

	if _, err := stream.Read(t.Context(), "HEAD:a.go"); !errors.Is(err, ErrClosed) {
		t.Fatalf("Read() after Close err = %v, want ErrClosed", err)
	}
}

func TestStreamCancelWhileBlocked(t *testing.T) {
	state := useFakeGit(t, "silent", "")
	stream := NewStream(t.TempDir())

	t.Cleanup(func() { _ = stream.Close() })

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	done := make(chan error, 1)

	go func() {
		_, err := stream.Read(ctx, "HEAD:a.go")
		done <- err
	}()

	// Opening the pipe blocks until git has taken the request, so Read is waiting on the reply.
	ready, err := os.OpenFile(filepath.Join(state, "ready"), os.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("open ready: %v", err)
	}

	t.Cleanup(func() { _ = ready.Close() })

	stream.state.Lock()
	pid := stream.proc.cmd.Process.Pid
	stream.state.Unlock()

	cancel()

	if err = <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Read() err = %v, want context.Canceled", err)
	}

	if stream.proc != nil {
		t.Fatal("the killed process was kept")
	}

	if !isReaped(pid) {
		t.Fatal("the killed process was not reaped")
	}
}

func TestStartRefusesACanceledContextWithoutLeaking(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	before := openDescriptors(t)

	for range 20 {
		if _, err := start(canceled, t.TempDir()); !errors.Is(err, context.Canceled) {
			t.Fatalf("start() err = %v, want context.Canceled", err)
		}
	}

	if after := openDescriptors(t); after > before {
		t.Fatalf("start leaked %d descriptors", after-before)
	}
}

func openDescriptors(t testing.TB) int {
	t.Helper()

	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Skipf("no /proc/self/fd: %v", err)
	}

	return len(entries)
}

func TestExchangeReplies(t *testing.T) {
	testCases := []struct {
		name       string
		reply      string
		want       string
		wantErr    error
		wantNotErr error
		isRetained bool
	}{
		{name: "blob", reply: "abc blob 3\nabc\n", want: "abc", isRetained: true},
		{
			name: "ambiguous", reply: "HEAD:a.go ambiguous\n",
			wantErr: ErrAmbiguous, wantNotErr: ErrMissing, isRetained: true,
		},
		{name: "missing", reply: "HEAD:a.go missing\n", wantErr: ErrMissing, isRetained: true},
		{
			name: "commit object", reply: "abc commit 5\nhello\n",
			wantErr: ErrNotBlob, wantNotErr: ErrProcess, isRetained: true,
		},
		{name: "garbage header", reply: "garbage\n", wantErr: errProtocol},
		{name: "text size", reply: "abc blob many\n", wantErr: errProtocol},
		{name: "negative size", reply: "abc blob -1\n", wantErr: errProtocol},
		{
			name: "absurd size", reply: "abc blob 99999999999999\n",
			wantErr: ErrTooLarge, wantNotErr: ErrProcess,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			useFakeGit(t, "reply", testCase.reply)

			stream := NewStream(t.TempDir())

			t.Cleanup(func() { _ = stream.Close() })

			// A second read proves the first left the pipe in step.
			for range 2 {
				got, err := stream.Read(t.Context(), "HEAD:a.go")
				if !errors.Is(err, testCase.wantErr) {
					t.Fatalf("Read() err = %v, want %v", err, testCase.wantErr)
				}

				if testCase.wantNotErr != nil && errors.Is(err, testCase.wantNotErr) {
					t.Fatalf("Read() err = %v, must not be %v", err, testCase.wantNotErr)
				}

				if string(got) != testCase.want {
					t.Fatalf("Read() = %q, want %q", got, testCase.want)
				}

				isKept := stream.proc != nil
				if isKept != testCase.isRetained {
					t.Fatalf("process kept = %t, want %t", isKept, testCase.isRetained)
				}

				if !isKept {
					return
				}
			}
		})
	}
}

func TestStreamDrainsAnOversizeBlob(t *testing.T) {
	useFakeGit(t, "oversize", "")

	stream := NewStream(t.TempDir())

	t.Cleanup(func() { _ = stream.Close() })

	_, err := stream.Read(t.Context(), "HEAD:big.bin")
	if !errors.Is(err, ErrTooLarge) || errors.Is(err, ErrProcess) {
		t.Fatalf("Read(oversize) err = %v, want ErrTooLarge and not ErrProcess", err)
	}

	got, err := stream.Read(t.Context(), "HEAD:a.go")
	if err != nil || string(got) != "abc" {
		t.Fatalf("Read() after the oversize blob = %q, %v; want abc", got, err)
	}
}
