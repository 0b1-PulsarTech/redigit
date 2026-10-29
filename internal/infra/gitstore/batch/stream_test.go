package batch

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestStreamRead(t *testing.T) {
	t.Parallel()

	repo := newRepo(t)
	commitFiles(
		t,
		repo,
		"one",
		map[string]string{"dir/a.go": "alpha\n", "empty.go": "", "sp ace.go": "gap\n"},
	)

	blob := strings.TrimSpace(gitIn(t, repo, "rev-parse", "HEAD:dir/a.go"))

	testCases := []struct {
		name    string
		spec    string
		want    string
		wantErr error
	}{
		{name: "rev and path", spec: "HEAD:dir/a.go", want: "alpha\n"},
		{name: "object id", spec: blob, want: "alpha\n"},
		{name: "empty blob", spec: "HEAD:empty.go", want: ""},
		{name: "missing path", spec: "HEAD:nope.go", wantErr: ErrMissing},
		{name: "spaced path", spec: "HEAD:sp ace.go", want: "gap\n"},
		{name: "missing spaced path", spec: "HEAD:no such file", wantErr: ErrMissing},
		{name: "missing oid", spec: strings.Repeat("0", 40), wantErr: ErrMissing},
		{name: "line break in spec", spec: "HEAD\nmain", wantErr: ErrBadSpec},
		{name: "empty spec", spec: "", wantErr: ErrBadSpec},
	}

	stream := NewStream(repo)

	t.Cleanup(func() { _ = stream.Close() })

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := stream.Read(t.Context(), testCase.spec)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Read() err = %v, want %v", err, testCase.wantErr)
			}

			if string(got) != testCase.want {
				t.Fatalf("Read() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestStreamReadAfterMissingStillServes(t *testing.T) {
	t.Parallel()

	repo := newRepo(t)
	commitFiles(t, repo, "one", map[string]string{"a.go": "alpha\n"})

	stream := NewStream(repo)

	t.Cleanup(func() { _ = stream.Close() })

	if _, err := stream.Read(t.Context(), "HEAD:gone.go"); !errors.Is(err, ErrMissing) {
		t.Fatalf("missing read err = %v", err)
	}

	got, err := stream.Read(t.Context(), "HEAD:a.go")
	if err != nil || string(got) != "alpha\n" {
		t.Fatalf("Read() = %q, %v", got, err)
	}
}

func TestStreamConcurrentReads(t *testing.T) {
	t.Parallel()

	repo := newRepo(t)
	files := map[string]string{}

	for _, name := range []string{"a.go", "b.go", "c.go", "d.go"} {
		files[name] = strings.Repeat(name+"\n", 5000)
	}

	commitFiles(t, repo, "one", files)

	stream := NewStream(repo)

	t.Cleanup(func() { _ = stream.Close() })

	start := make(chan struct{})

	var (
		group  sync.WaitGroup
		failed = make(chan string, 64)
	)

	for range 16 {
		group.Go(func() {
			<-start

			for name, want := range files {
				got, err := stream.Read(t.Context(), "HEAD:"+name)
				if err != nil || string(got) != want {
					failed <- name + ": mixed or failed reply"
				}
			}
		})
	}

	close(start)
	group.Wait()
	close(failed)

	for message := range failed {
		t.Error(message)
	}
}

func TestStreamCancelAndKill(t *testing.T) {
	t.Parallel()

	repo := newRepo(t)
	commitFiles(t, repo, "one", map[string]string{"a.go": "alpha\n"})

	testCases := []struct {
		name    string
		disrupt func(stream *Stream) context.Context
		wantErr error
	}{
		{
			name: "canceled before read",
			disrupt: func(*Stream) context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()

				return ctx
			},
			wantErr: context.Canceled,
		},
		{
			name: "process killed",
			disrupt: func(stream *Stream) context.Context {
				stream.state.Lock()
				stream.proc.kill()
				stream.state.Unlock()

				return context.Background()
			},
			wantErr: ErrProcess,
		},
		{
			name: "closed stream",
			disrupt: func(stream *Stream) context.Context {
				_ = stream.Close()

				return context.Background()
			},
			wantErr: ErrClosed,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			stream := NewStream(repo)

			t.Cleanup(func() { _ = stream.Close() })

			if _, err := stream.Read(t.Context(), "HEAD:a.go"); err != nil {
				t.Fatalf("warm-up read: %v", err)
			}

			ctx := testCase.disrupt(stream)

			_, err := stream.Read(ctx, "HEAD:a.go")
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Read() err = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestStreamRecoversAfterKill(t *testing.T) {
	t.Parallel()

	repo := newRepo(t)
	commitFiles(t, repo, "one", map[string]string{"a.go": "alpha\n"})

	stream := NewStream(repo)

	t.Cleanup(func() { _ = stream.Close() })

	if _, err := stream.Read(t.Context(), "HEAD:a.go"); err != nil {
		t.Fatalf("warm-up read: %v", err)
	}

	stream.proc.kill()

	if _, err := stream.Read(t.Context(), "HEAD:a.go"); !errors.Is(err, ErrProcess) {
		t.Fatalf("read on killed process err = %v, want ErrProcess", err)
	}

	got, err := stream.Read(t.Context(), "HEAD:a.go")
	if err != nil || string(got) != "alpha\n" {
		t.Fatalf("read after respawn = %q, %v", got, err)
	}
}

func TestStreamStartFailure(t *testing.T) {
	t.Parallel()

	stream := NewStream(t.TempDir())

	t.Cleanup(func() { _ = stream.Close() })

	if _, err := stream.Read(t.Context(), "HEAD:a.go"); !errors.Is(err, ErrProcess) {
		t.Fatalf("Read() outside a repo err = %v, want ErrProcess", err)
	}
}

func gitIn(t testing.TB, dir string, args ...string) string {
	t.Helper()

	out, err := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...).
		CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}

	return string(out)
}

func newRepo(t testing.TB) string {
	t.Helper()

	dir := t.TempDir()
	gitIn(t, dir, "init", "-q", "-b", "main")
	gitIn(t, dir, "config", "user.email", "tester@example.com")
	gitIn(t, dir, "config", "user.name", "Tester")

	return dir
}

func commitFiles(t testing.TB, dir, message string, files map[string]string) {
	t.Helper()

	for name, content := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", message)
}
