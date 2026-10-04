package chunk

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// A fixture proves a chunker against the file its author imagined; this
// proves it against a real repository's every go.mod, go.work and query
// file, which is where the require/replace collision came from. Skipped
// unless REDIGIT_REAL_REPO names a checkout.
func TestChunkersAgainstARealRepository(t *testing.T) {
	t.Parallel()

	repo := os.Getenv("REDIGIT_REAL_REPO")
	if repo == "" {
		t.Skip("REDIGIT_REAL_REPO is unset")
	}

	rev := os.Getenv("REDIGIT_REAL_REV")
	if rev == "" {
		rev = "HEAD"
	}

	out, err := exec.CommandContext(t.Context(), "git", "-C", repo, "ls-tree", "-r", "--name-only", rev).
		Output()
	if err != nil {
		t.Fatalf("list %s: %v", rev, err)
	}

	var checked int

	for path := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		chunker := pick(path)
		if chunker == nil {
			continue
		}

		source, showErr := exec.CommandContext(t.Context(), "git", "-C", repo, "show", rev+":"+path).
			Output()
		if showErr != nil {
			t.Fatalf("read %s: %v", path, showErr)
		}

		decls, chunkErr := chunker.Chunks(context.Background(), path, source)
		if chunkErr != nil {
			t.Fatalf("chunk %s: %v", path, chunkErr)
		}

		assertNoDuplicateKeys(t, decls)
		assertSpansCoverSource(t, decls, source)

		checked++
	}

	if checked == 0 {
		t.Fatal("no go.mod, go.work or query file was checked: the run proves nothing")
	}

	t.Logf("checked %d files at %s", checked, rev)
}

// pick returns the chunker Registry would choose for path, minus WholeFile:
// a file that only WholeFile claims has one Decl covering everything and
// cannot fail either assertion, so including it would inflate the count with
// checks that cannot fail.
func pick(path string) interface {
	Chunks(ctx context.Context, path string, source []byte) ([]entities.Decl, error)
} {
	switch {
	case (GoMod{}).Handles(path):
		return GoMod{}
	case (GoWork{}).Handles(path):
		return GoWork{}
	case (SQLQueries{}).Handles(path):
		return SQLQueries{}
	default:
		return nil
	}
}
