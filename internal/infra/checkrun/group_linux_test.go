package checkrun

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func isGone(pid int) bool {
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return true
	}

	_, afterName, _ := strings.Cut(string(stat), ") ")

	return strings.HasPrefix(afterName, "Z")
}

func TestCheckPassesAndReapsAChildHoldingTheOutput(t *testing.T) {
	t.Parallel()

	pidFile := filepath.Join(t.TempDir(), "pid")
	runner, slotDir := runnerFor(t, fake("orphan", pidFile))

	outcome, err := runner.Check(context.Background(), slotDir, goUnit())
	if err != nil || outcome.IsFailed {
		t.Fatalf("check: outcome = %+v err = %v, want a pass", outcome, err)
	}

	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("read pid: %v", err)
	}

	pid, err := strconv.Atoi(string(raw))
	if err != nil {
		t.Fatalf("pid %q: %v", raw, err)
	}

	t.Cleanup(func() {
		if process, findErr := os.FindProcess(pid); findErr == nil {
			_ = process.Kill()
		}
	})

	deadline := time.Now().Add(10 * time.Second)
	for !isGone(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("child %d outlived the step", pid)
		}
	}
}
