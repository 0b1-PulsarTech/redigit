//go:build unix

package format

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestArgBudgetValue(t *testing.T) {
	const want = 30000

	if argBudget != want {
		t.Errorf("argBudget = %d, want %d", argBudget, want)
	}
}

func newFifo(t *testing.T) string {
	t.Helper()

	name := filepath.Join(t.TempDir(), "started")
	if err := syscall.Mkfifo(name, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("FAKEFMT_FIFO", name)

	return name
}

func awaitEOF(t *testing.T, reader *os.File) {
	t.Helper()

	drained := make(chan struct{})

	go func() {
		defer close(drained)

		_, _ = io.Copy(io.Discard, reader)
	}()

	select {
	case <-drained:
		_ = reader.Close()
	case <-time.After(30 * time.Second):
		t.Fatal("a process still holds the fifo")
	}
}

func TestCancelMidRun(t *testing.T) {
	testCases := []struct {
		name  string
		mode  Mode
		files []File
	}{
		{"stdin mode", ModeStdin, numbered(2)},
		{"files mode", ModeFiles, numbered(3)},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fifo := newFifo(t)
			t.Setenv("FAKEFMT_HANG", "1")

			config := fakeConfig(testCase.mode, 5*time.Minute)
			config.Workers = 1

			engine, err := New(config)
			if err != nil {
				t.Fatal(err)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			type outcome struct {
				files    []File
				failures []*Failure
			}

			finished := make(chan outcome, 1)

			go func() {
				files, failures := engine.FormatAll(ctx, testCase.files)
				finished <- outcome{files, failures}
			}()

			reader, err := os.OpenFile(fifo, os.O_RDONLY, 0)
			if err != nil {
				t.Fatal(err)
			}

			begin := time.Now()

			cancel()

			got := <-finished

			if elapsed := time.Since(begin); elapsed > time.Minute/2 {
				t.Errorf("returned %s after the cancel", elapsed)
			}

			if len(got.failures) != len(testCase.files) {
				t.Fatalf("failures = %v, want one per file", got.failures)
			}

			for index, failure := range got.failures {
				if !errors.Is(failure, context.Canceled) {
					t.Errorf("failure %d = %v, want context.Canceled", index, failure)
				}

				if string(got.files[index].Content) != string(testCase.files[index].Content) {
					t.Errorf("file %d changed to %q", index, got.files[index].Content)
				}
			}

			awaitEOF(t, reader)
		})
	}
}

func TestPlantedFifoIsRefused(t *testing.T) {
	t.Setenv("FAKEFMT_PLANTFIFO", "1")

	engine, err := New(fakeConfig(ModeFiles, time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	type outcome struct {
		files    []File
		failures []*Failure
	}

	finished := make(chan outcome, 1)

	go func() {
		files, failures := engine.FormatAll(context.Background(), []File{{"a.go", []byte("a")}})
		finished <- outcome{files, failures}
	}()

	select {
	case got := <-finished:
		if len(got.failures) != 1 || string(got.files[0].Content) != "a" {
			t.Fatalf(
				"failures = %v, files = %q, want the original and one failure",
				got.failures,
				got.files,
			)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the read-back blocked on a planted fifo")
	}
}

func TestReadCappedKinds(t *testing.T) {
	testCases := []struct {
		name    string
		plant   func(t *testing.T, dir string)
		want    string
		wantErr error
	}{
		{
			name:  "a regular file is read",
			plant: func(t *testing.T, dir string) { writeFile(t, filepath.Join(dir, "out")) },
			want:  "ok",
		},
		{
			name:    "a fifo is refused without blocking",
			plant:   func(t *testing.T, dir string) { mkfifo(t, filepath.Join(dir, "out")) },
			wantErr: ErrOutputKind,
		},
		{
			name: "a link to a fifo inside the root is refused without blocking",
			plant: func(t *testing.T, dir string) {
				mkfifo(t, filepath.Join(dir, "pipe"))
				symlink(t, "pipe", filepath.Join(dir, "out"))
			},
			wantErr: ErrOutputKind,
		},
		{
			name: "a link to a regular file inside the root is judged by its target",
			plant: func(t *testing.T, dir string) {
				writeFile(t, filepath.Join(dir, "real"))
				symlink(t, "real", filepath.Join(dir, "out"))
			},
			want: "ok",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			dir := t.TempDir()
			testCase.plant(t, dir)

			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}

			defer root.Close()

			type outcome struct {
				content []byte
				err     error
			}

			finished := make(chan outcome, 1)

			go func() {
				content, readErr := readCapped(root, "out", 1<<10)
				finished <- outcome{content, readErr}
			}()

			select {
			case got := <-finished:
				if !errors.Is(got.err, testCase.wantErr) || string(got.content) != testCase.want {
					t.Errorf(
						"readCapped = %q, %v, want %q, %v",
						got.content,
						got.err,
						testCase.want,
						testCase.wantErr,
					)
				}
			case <-time.After(30 * time.Second):
				t.Fatal("readCapped blocked")
			}
		})
	}
}

func writeFile(t *testing.T, name string) {
	t.Helper()

	if err := os.WriteFile(name, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mkfifo(t *testing.T, name string) {
	t.Helper()

	if err := syscall.Mkfifo(name, 0o600); err != nil {
		t.Fatal(err)
	}
}

func symlink(t *testing.T, target, name string) {
	t.Helper()

	if err := os.Symlink(target, name); err != nil {
		t.Fatal(err)
	}
}
