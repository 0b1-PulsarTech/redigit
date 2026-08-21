//go:build integration

package gitstore

import (
	"testing"
	"time"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/rewriting"
)

// TestHistoryRebuildsGrowingFiles is the end-to-end proof of the write half:
// a file that grows across two commits, an executable that keeps its bit, and a
// final tree that matches the target exactly.
func TestHistoryRebuildsGrowingFiles(t *testing.T) {
	dir := newTestRepo(t)
	commitTestFiles(t, dir, "target", map[string][]byte{
		"a.go":   []byte("One\nTwo\n"),
		"run.sh": []byte("#!/bin/sh\n"),
	})
	runTestGit(t, dir, "update-index", "--chmod=+x", "run.sh")
	runTestGit(t, dir, "commit", "-q", "-m", "make it executable")

	ctx := t.Context()
	author := Identity{
		Name:  "Tester",
		Email: "tester@example.com",
		When:  time.Unix(1000000000, 0).UTC(),
	}
	history := NewHistory(dir, "HEAD", author)

	ref, err := history.Write(ctx, "refs/heads/rebuilt", []rewriting.Commit{
		{Subject: "feat: One", Writes: []rewriting.FileContent{
			{Path: "a.go", Content: []byte("One\n")},
		}},
		{Subject: "feat: Two", Writes: []rewriting.FileContent{
			{Path: "a.go", Content: []byte("One\nTwo\n")},
			{Path: "run.sh", Content: []byte("#!/bin/sh\n")},
		}},
	})
	if err != nil {
		t.Fatalf("Write(): %v", err)
	}

	if err = history.Verify(ctx, ref, "HEAD"); err != nil {
		t.Fatalf("final-tree invariant: %v", err)
	}
}

// TestHistoryIsReproducible pins that the same plan written twice produces the
// same identifier. Without it a comparison between two rebuild rounds compares
// noise, which is exactly the mistake of comparing commit hashes instead of
// trees, one level up.
func TestHistoryIsReproducible(t *testing.T) {
	dir := newTestRepo(t)
	commitTestFiles(t, dir, "target", map[string][]byte{"a.go": []byte("One\n")})

	ctx := t.Context()
	author := Identity{
		Name:  "Tester",
		Email: "tester@example.com",
		When:  time.Unix(1000000000, 0).UTC(),
	}

	commits := []rewriting.Commit{{
		Subject: "feat: One",
		Writes:  []rewriting.FileContent{{Path: "a.go", Content: []byte("One\n")}},
	}}

	first, err := NewHistory(dir, "HEAD", author).Write(ctx, "refs/heads/one", commits)
	if err != nil {
		t.Fatalf("first Write(): %v", err)
	}

	second, err := NewHistory(dir, "HEAD", author).Write(ctx, "refs/heads/two", commits)
	if err != nil {
		t.Fatalf("second Write(): %v", err)
	}

	if first != second {
		t.Fatalf("rebuild is not reproducible: %s then %s", first, second)
	}
}
