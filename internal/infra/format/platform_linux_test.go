//go:build linux

package format

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestTimeoutKillsTheWholeGroup(t *testing.T) {
	testCases := []struct {
		name string
		mode Mode
	}{
		{"stdin mode", ModeStdin},
		{"files mode", ModeFiles},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			pidFile := filepath.Join(t.TempDir(), "grandchild.pid")
			t.Setenv("FAKEFMT_PIDFILE", pidFile)

			engine, err := New(fakeConfig(testCase.mode, time.Second))
			if err != nil {
				t.Fatal(err)
			}

			start := time.Now()
			_, failures := engine.FormatAll(
				context.Background(),
				[]File{{Path: "a.go", Content: []byte("SPAWN")}},
			)

			if len(failures) != 1 {
				t.Fatalf("failures = %v, want the timeout", failures)
			}

			if elapsed := time.Since(start); elapsed > time.Minute/2 {
				t.Errorf("returned after %s, the group was not killed", elapsed)
			}

			raw, err := os.ReadFile(pidFile)
			if err != nil {
				t.Fatalf("the tool never spawned its grandchild: %v", err)
			}

			if !awaitGone(t, strings.TrimSpace(string(raw))) {
				t.Errorf("grandchild %s outlived the timeout", raw)
			}
		})
	}
}

func awaitGone(t *testing.T, pid string) bool {
	t.Helper()

	deadline := time.After(30 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)

	defer ticker.Stop()

	for running(t, pid) {
		select {
		case <-deadline:
			return false
		case <-ticker.C:
		}
	}

	return true
}

func running(t *testing.T, pid string) bool {
	t.Helper()

	if _, err := strconv.Atoi(pid); err != nil {
		t.Fatalf("pid %q: %v", pid, err)
	}

	stat, err := os.ReadFile("/proc/" + pid + "/stat")
	if err != nil {
		return false
	}

	fields := strings.Fields(string(stat[strings.LastIndexByte(string(stat), ')')+1:]))

	return len(fields) > 0 && fields[0] != "Z" && fields[0] != "X"
}

func TestBackgroundChildIsReaped(t *testing.T) {
	testCases := []struct {
		name string
		mode Mode
	}{
		{"stdin mode", ModeStdin},
		{"files mode", ModeFiles},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fifo := newFifo(t)

			reader, err := os.OpenFile(fifo, os.O_RDONLY|syscall.O_NONBLOCK, 0)
			if err != nil {
				t.Fatal(err)
			}

			engine, err := New(fakeConfig(testCase.mode, time.Minute))
			if err != nil {
				t.Fatal(err)
			}

			_, failures := engine.FormatAll(
				context.Background(),
				[]File{{Path: "a.go", Content: []byte("BGCHILD")}},
			)
			if len(failures) != 0 {
				t.Fatalf("failures = %v, the tool itself succeeded", failures)
			}

			if err = syscall.SetNonblock(int(reader.Fd()), false); err != nil {
				t.Fatal(err)
			}

			awaitEOF(t, reader)
		})
	}
}
