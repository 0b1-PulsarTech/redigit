//go:build integration

package gitstore

import (
	"errors"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
)

func TestCheckMatchingTrees(t *testing.T) {
	t.Parallel()

	dir := newTestRepo(t)
	commitTestFiles(t, dir, "only", map[string][]byte{"a.go": []byte("package a\n")})
	runTestGit(t, dir, "update-ref", "refs/heads/other", "main")

	if err := Check(t.Context(), dir, "main", "refs/heads/other"); err != nil {
		t.Fatalf("Check() err = %v, want nil for identical trees", err)
	}
}

func TestCheckDivergingTrees(t *testing.T) {
	t.Parallel()

	dir := newTestRepo(t)
	commitTestFiles(t, dir, "base", map[string][]byte{"a.go": []byte("package a\n")})
	runTestGit(t, dir, "branch", "target")
	commitTestFiles(t, dir, "extra", map[string][]byte{"b.go": []byte("package b\n")})

	err := Check(t.Context(), dir, "main", "refs/heads/target")
	if err == nil {
		t.Fatalf("Check() err = nil, want an invariant error for diverging trees")
	}

	if !errors.Is(err, apperr.ErrInvariant) {
		t.Fatalf("Check() err = %v, want it to wrap apperr.ErrInvariant", err)
	}

	var invariantErr *InvariantError
	if !errors.As(err, &invariantErr) {
		t.Fatalf("Check() err = %v, want an *InvariantError", err)
	}

	if len(invariantErr.Diverging) != 1 || invariantErr.Diverging[0] != "b.go" {
		t.Fatalf("InvariantError.Diverging = %v, want [b.go]", invariantErr.Diverging)
	}
}
