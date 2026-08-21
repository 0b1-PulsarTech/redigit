//go:build integration

package gitstore

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestWriteRoundTripPreservesModes is the proof the tool can rebuild a real
// repository. A tree hashes the mode with the content, so an executable script
// rebuilt as a plain file produces a different tree; before modes travelled
// with the write, this failed on the final-tree invariant with no path named.
func TestWriteRoundTripPreservesModes(t *testing.T) {
	dir := newTestRepo(t)

	script := filepath.Join(dir, "run.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	if err := os.Symlink("run.sh", filepath.Join(dir, "link.sh")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "plain.txt"), []byte("x\n"), 0o600); err != nil {
		t.Fatalf("write plain: %v", err)
	}

	runTestGit(t, dir, "add", "-A")
	runTestGit(t, dir, "commit", "-q", "-m", "target")

	ctx := t.Context()
	reader := NewReader(dir, "HEAD")

	modes, err := reader.Modes(ctx)
	if err != nil {
		t.Fatalf("Modes(): %v", err)
	}

	testCases := []struct {
		name string
		path string
		want FileMode
	}{
		{"executable keeps its bit", "run.sh", ModeExecutable},
		{"symlink is its own mode", "link.sh", ModeSymlink},
		{"plain file", "plain.txt", ModeFile},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := modes[testCase.path]; got != testCase.want {
				t.Fatalf("mode of %s = %q, want %q", testCase.path, got, testCase.want)
			}
		})
	}

	snap, err := reader.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot(): %v", err)
	}

	writes := make([]FileWrite, 0, len(snap.Files))

	for _, path := range snap.Files {
		content, readErr := reader.Read(ctx, path)
		if readErr != nil {
			t.Fatalf("read %s: %v", path, readErr)
		}

		writes = append(writes, FileWrite{Path: path, Mode: modes[path], Content: content})
	}

	when := time.Unix(1000000000, 0).UTC()
	id := Identity{Name: "Tester", Email: "tester@example.com", When: when}

	if _, err = NewWriter(dir).Write(ctx, "refs/heads/rebuilt", []WriteCommit{{
		Author:    id,
		Committer: id,
		Message:   "rebuild",
		Writes:    writes,
	}}); err != nil {
		t.Fatalf("Write(): %v", err)
	}

	if err = Check(ctx, dir, "refs/heads/rebuilt", "HEAD"); err != nil {
		t.Fatalf("final-tree invariant: %v", err)
	}
}

// TestSnapshotRefusesASubmodule proves the reader names the offending path
// instead of silently rebuilding a submodule pointer as a file.
func TestSnapshotRefusesASubmodule(t *testing.T) {
	outer := newTestRepo(t)
	inner := newTestRepo(t)

	commitTestFiles(t, inner, "inner", map[string][]byte{"a.txt": []byte("a\n")})

	cmd := exec.CommandContext(
		t.Context(), "git", "-C", outer,
		"-c", "protocol.file.allow=always",
		"submodule", "add", "-q", inner, "sub",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("submodule add unavailable here: %v\n%s", err, out)
	}

	runTestGit(t, outer, "commit", "-q", "-m", "add submodule")

	if _, err := NewReader(outer, "HEAD").Modes(t.Context()); err == nil {
		t.Fatal("Modes() accepted a submodule instead of naming it")
	}
}
