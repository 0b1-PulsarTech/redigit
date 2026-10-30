package gitstore

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"os"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore/batch"
)

func (reader Reader) Changes(ctx context.Context) iter.Seq2[indexing.RevisionChange, error] {
	return func(yield func(indexing.RevisionChange, error) bool) {
		for commit, err := range batch.Log(ctx, reader.RepoPath, reader.Ref) {
			if err != nil {
				yield(indexing.RevisionChange{}, fmt.Errorf("read log: %w", err))

				return
			}

			change := indexing.RevisionChange{
				Rev:   commit.OID,
				Files: make([]indexing.ChangedFile, 0, len(commit.Entries)),
			}

			for _, entry := range commit.Entries {
				change.Files = append(change.Files, indexing.ChangedFile{
					Path: entry.NewPath, Blob: entry.NewBlob, IsDeleted: entry.IsDelete(),
				})
			}

			if !yield(change, nil) {
				return
			}
		}
	}
}

func (reader Reader) ReadBlob(ctx context.Context, blob string) ([]byte, error) {
	out, err := reader.read(ctx, blob)
	isUnreadable := errors.Is(err, batch.ErrMissing) || errors.Is(err, batch.ErrNotBlob) ||
		errors.Is(err, batch.ErrTooLarge)

	if isUnreadable {
		return nil, fmt.Errorf("read blob: %w: %w", indexing.ErrBlobMissing, err)
	}

	if err != nil {
		return nil, fmt.Errorf("read blob: %w", err)
	}

	return out, nil
}

// Checkout is a throwaway worktree holding one revision.
//
// It exists because a language server reads the WORKING TREE, not the
// revision anyone asked about. Indexing a revision that is not checked out
// silently mismatches: positions computed from the revision's text are sent to
// a server reading different bytes, and the answers are wrong without any
// error saying so. Materialising the revision is what makes "any revision"
// true rather than "whatever happens to be checked out".
type Checkout struct {
	repoPath string
	dir      string
}

// NewCheckout materialises rev in a temporary worktree. The caller must Close
// it; a leaked worktree stays registered in the repository until pruned.
func NewCheckout(ctx context.Context, repoPath, rev string) (*Checkout, error) {
	dir, err := os.MkdirTemp("", "redigit-checkout-")
	if err != nil {
		return nil, fmt.Errorf("create checkout directory: %w", err)
	}

	// --detach because this worktree is read-only scaffolding, not a branch
	// anyone will commit on, and --force because the same revision may already
	// be checked out elsewhere in the repository.
	if _, err = runGit(
		ctx,
		repoPath,
		"worktree",
		"add",
		"--detach",
		"--force",
		dir,
		rev,
	); err != nil {
		_ = os.RemoveAll(dir)

		return nil, fmt.Errorf("check out %s: %w", rev, err)
	}

	return &Checkout{repoPath: repoPath, dir: dir}, nil
}

// Dir is the directory holding the revision.
func (c *Checkout) Dir() string { return c.dir }

// Close removes the worktree and its directory.
func (c *Checkout) Close(ctx context.Context) error {
	if _, err := runGit(ctx, c.repoPath, "worktree", "remove", "--force", c.dir); err != nil {
		// The directory is removed anyway: leaving it behind on a failed
		// unregister turns one leak into two.
		_ = os.RemoveAll(c.dir)

		return fmt.Errorf("remove checkout: %w", err)
	}

	return nil
}

func ResolveCommit(ctx context.Context, repoPath, rev string) (string, error) {
	if strings.HasPrefix(rev, "-") {
		return "", fmt.Errorf("%w: a rev may not start with a dash", ErrBadRev)
	}

	// The commit oid, never the tree: a rebuilt history shares its source tree.
	return revParse(ctx, repoPath, rev+"^{commit}")
}

func CheckRef(ref string) error {
	refused := !strings.HasPrefix(ref, "refs/") ||
		strings.ContainsAny(ref, " ~^:?*[\\") ||
		strings.IndexFunc(ref, isControlByte) >= 0 ||
		strings.Contains(ref, "..") || strings.Contains(ref, "@{") ||
		strings.HasSuffix(ref, ".")

	// Per component git refuses an empty one, a leading dot and a .lock suffix; a lone @
	// is invalid only as a whole refname, which the refs/ prefix already rules out.
	for component := range strings.SplitSeq(ref, "/") {
		refused = refused || component == "" || component[0] == '.' ||
			strings.HasSuffix(component, ".lock")
	}

	if refused {
		return ErrBadRef
	}

	return nil
}

func isControlByte(char rune) bool { return char < ' ' || char == 0x7f }

// PathConflictError is a commit whose Writes and Deletes name the same path.
type PathConflictError struct {
	Path string
}

// UnsupportedModeError is a write whose mode git will not accept for a blob.
// It is refused at encode time rather than handed to fast-import, which would
// reject the whole stream with a parse error naming no path.
type UnsupportedModeError struct {
	Path string
	Mode FileMode
}

func (e *UnsupportedModeError) Error() string {
	return fmt.Sprintf("unsupported file mode %q for %q", e.Mode, e.Path)
}

func (e *UnsupportedModeError) Unwrap() error { return errUnsupportedMode }

func (e *PathConflictError) Error() string {
	return "path both written and deleted in the same commit"
}

var (
	errPathConflict    = errors.New("path conflict")
	errUnsupportedMode = errors.New("unsupported file mode")
)

// Unwrap lets a caller match this family of error with errors.Is without
// depending on the exact Path it carries.
func (e *PathConflictError) Unwrap() error { return errPathConflict }
