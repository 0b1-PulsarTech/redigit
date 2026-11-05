package gitstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore/batch"
)

type treeEntry struct {
	path string
	mode FileMode
}

// entries parses `ls-tree -r -z`, whose records are
// "<mode> SP <type> SP <object> TAB <path>". A submodule (160000) is not a blob
// and is reported rather than silently rebuilt as a file.
func (reader Reader) entries(ctx context.Context) ([]treeEntry, error) {
	out, err := runGit(ctx, reader.RepoPath, "ls-tree", "-r", "-z", reader.Ref)
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
func (reader Reader) history(ctx context.Context) ([]entities.SourceTouch, error) {
	out, err := runGit(ctx, reader.RepoPath,
		"log", "--reverse", "--name-only", "-z", "--pretty=format:%x01%H", reader.Ref)
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

func (reader Reader) read(ctx context.Context, spec string) ([]byte, error) {
	if reader.stream == nil {
		return nil, fmt.Errorf("zero reader, build it with NewReader: %w", batch.ErrClosed)
	}

	// The batch protocol is line based, so a path holding an LF takes a one-shot read.
	if strings.Contains(spec, "\n") {
		return reader.readOnce(ctx, spec, batch.MaxObject)
	}

	out, err := reader.stream.Read(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("read object: %w", err)
	}

	return out, nil
}

func (reader Reader) readOnce(ctx context.Context, spec string, limit int64) ([]byte, error) {
	args := []string{"cat-file", "blob", "--end-of-options", spec}
	out, err := runGitCapped(ctx, reader.RepoPath, limit, args...)

	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, fmt.Errorf("read object: %w", ctxErr)
	}

	if errors.Is(err, batch.ErrTooLarge) {
		return nil, fmt.Errorf("read object: %w", err)
	}

	if cmdErr, isCommand := errors.AsType[*commandError](err); isCommand {
		return nil, fmt.Errorf("read object: %w", classifyCatFile(cmdErr))
	}

	if err != nil {
		return nil, fmt.Errorf("read object: %w: %w", batch.ErrProcess, err)
	}

	return out, nil
}

func classifyCatFile(cmdErr *commandError) error {
	// Only a plain non-zero exit carries git's verdict; a signal reports -1.
	exitErr, isExit := errors.AsType[*exec.ExitError](cmdErr)
	if !isExit || exitErr.ExitCode() < 1 {
		return fmt.Errorf("%w: %w", batch.ErrProcess, cmdErr)
	}

	stderr := strings.ToLower(cmdErr.Stderr)

	// git echoes the path, so the missing-path shape is read before any text a path could hold.
	isPathAbsent := strings.HasPrefix(stderr, "fatal: path ") &&
		(strings.Contains(stderr, "does not exist in") ||
			strings.Contains(stderr, "exists on disk, but not in"))

	switch {
	case isPathAbsent,
		strings.HasPrefix(stderr, "fatal: not a valid object name"),
		strings.HasPrefix(stderr, "fatal: invalid object name"):
		return fmt.Errorf("%w: %w", batch.ErrMissing, cmdErr)
	case hasAmbiguityLine(stderr):
		return fmt.Errorf("%w: %w", batch.ErrAmbiguous, cmdErr)
	case strings.HasSuffix(strings.TrimSpace(stderr), "bad file"):
		return fmt.Errorf("%w: %w", batch.ErrNotBlob, cmdErr)
	}

	return fmt.Errorf("%w: %w: %s", batch.ErrProcess, cmdErr, cmdErr.Stderr)
}

func hasAmbiguityLine(stderr string) bool {
	for line := range strings.Lines(stderr) {
		isShortID := strings.HasPrefix(line, "error: short object id")
		if isShortID && strings.Contains(line, "is ambiguous") {
			return true
		}
	}

	return false
}
