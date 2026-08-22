package gitstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

var (
	_ indexing.Tree         = Reader{}
	_ indexing.RevisionTree = Reader{}
)

// ErrAbsentAtRevision reports a path a revision does not hold.
var ErrAbsentAtRevision = errors.New("path absent at revision")

// Reader reads a Snapshot out of a real git repository at Ref: the final
// tree's file list, the source history's per-position touch list, and a
// path's final content.
type Reader struct {
	RepoPath string
	Ref      string
}

func NewReader(repoPath, ref string) Reader { return Reader{RepoPath: repoPath, Ref: ref} }

// Snapshot reads the final tree's files and the source history at Ref.
func (r Reader) Snapshot(ctx context.Context) (entities.Snapshot, error) {
	files, err := r.finalFiles(ctx)
	if err != nil {
		return entities.Snapshot{}, fmt.Errorf("list final tree: %w", err)
	}

	history, err := r.history(ctx)
	if err != nil {
		return entities.Snapshot{}, fmt.Errorf("read source history: %w", err)
	}

	return entities.Snapshot{Root: r.RepoPath, Files: files, History: history}, nil
}

// Read returns path's content in the final tree at Ref, which is what every
// intermediate render is cut from rather than an intermediate blob.
func (r Reader) Read(ctx context.Context, path string) ([]byte, error) {
	out, err := runGit(ctx, r.RepoPath, "cat-file", "-p", r.Ref+":"+path)
	if err != nil {
		return nil, fmt.Errorf("read path content: %w", err)
	}

	return out, nil
}

// Modes reports the blob mode of every path in the final tree. A tree hashes
// the mode with the content, so rebuilding an executable script as a plain file
// yields a different tree and the final-tree invariant fails at the end without
// naming why.
func (r Reader) Modes(ctx context.Context) (map[string]FileMode, error) {
	entries, err := r.entries(ctx)
	if err != nil {
		return nil, err
	}

	out := make(map[string]FileMode, len(entries))
	for _, entry := range entries {
		out[entry.path] = entry.mode
	}

	return out, nil
}

// ReadAt returns a path's content at one revision. A path the revision does
// not hold reports ErrAbsentAtRevision rather than an empty file, so a caller
// scanning history can tell "deleted here" from "empty here".
func (r Reader) ReadAt(ctx context.Context, rev, path string) ([]byte, error) {
	out, err := runGit(ctx, r.RepoPath, "cat-file", "-p", rev+":"+path)
	if err != nil {
		return nil, fmt.Errorf("%s at %s: %w", path, rev, ErrAbsentAtRevision)
	}

	return out, nil
}

func (r Reader) finalFiles(ctx context.Context) ([]string, error) {
	entries, err := r.entries(ctx)
	if err != nil {
		return nil, err
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		files = append(files, entry.path)
	}

	slices.Sort(files)

	return files, nil
}

type treeEntry struct {
	path string
	mode FileMode
}

// entries parses `ls-tree -r -z`, whose records are
// "<mode> SP <type> SP <object> TAB <path>". A submodule (160000) is not a blob
// and is reported rather than silently rebuilt as a file.
func (r Reader) entries(ctx context.Context) ([]treeEntry, error) {
	out, err := runGit(ctx, r.RepoPath, "ls-tree", "-r", "-z", r.Ref)
	if err != nil {
		return nil, fmt.Errorf("list tree: %w", err)
	}

	records := splitNUL(out)
	entries := make([]treeEntry, 0, len(records))

	for _, record := range records {
		meta, path, found := strings.Cut(record, "\t")
		if !found {
			return nil, fmt.Errorf("%w: tree record %q has no path", apperr.ErrUnresolved, record)
		}

		mode, _, _ := strings.Cut(meta, " ")

		entry := treeEntry{path: path, mode: FileMode(mode)}
		if !entry.mode.Valid() {
			return nil, fmt.Errorf(
				"%w: %s carries mode %s, which is not a blob",
				apperr.ErrUnresolved, path, mode,
			)
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// simplification: plain --name-only omits the touched-path list for merge
// commits (git needs -m/--first-parent for that). A linear source history has
// none; a merge commit here surfaces as a position with an empty Touched, not
// a read failure, which is the ceiling until a caller needs merge histories.
func (r Reader) history(ctx context.Context) ([]entities.SourceTouch, error) {
	out, err := runGit(ctx, r.RepoPath,
		"log", "--reverse", "--name-only", "-z", "--pretty=format:%x01%H", r.Ref)
	if err != nil {
		return nil, fmt.Errorf("read log: %w", err)
	}

	return parseHistory(out), nil
}

// parseHistory splits a `git log --reverse --name-only -z --pretty=format:%x01%H`
// stream into one SourceTouch per commit, in the order git printed them (the
// reversed, chronological order). Each commit's chunk starts with its sha, an
// LF, then its touched paths NUL-terminated; the leading \x01 byte before
// each sha is what makes a commit's chunk boundary unambiguous even though
// paths may themselves contain LF-adjacent bytes.
func parseHistory(out []byte) []entities.SourceTouch {
	chunks := bytes.Split(out, []byte{1})
	history := make([]entities.SourceTouch, 0, len(chunks))

	for _, chunk := range chunks {
		if len(chunk) == 0 {
			continue
		}

		sha, rest, _ := bytes.Cut(chunk, []byte{'\n'})

		history = append(history, entities.SourceTouch{
			Position: entities.Position(len(history)),
			Rev:      string(bytes.TrimSpace(sha)),
			Touched:  splitNUL(rest),
		})
	}

	return history
}
