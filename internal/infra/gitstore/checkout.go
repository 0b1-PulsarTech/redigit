package gitstore

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/rewriting"
)

func openStage(ctx context.Context, repoPath, ref string) (rewriting.Staged, error) {
	if err := CheckRef(ref); err != nil {
		return rewriting.Staged{}, fmt.Errorf("stage history: %w", err)
	}

	// Scratch refs live under refs/redigit/, so a target there could be deleted with them.
	if strings.HasPrefix(ref+"/", "refs/redigit/") {
		return rewriting.Staged{}, fmt.Errorf("stage history: %w: scratch namespace", ErrBadRef)
	}

	// for-each-ref matches by prefix up to a slash, so the lines below the target show too.
	const format = "--format=%(refname)%00%(objectname)%00%(worktreepath)%00%(symref)"

	out, err := runGit(ctx, repoPath, "for-each-ref", format, ref)
	if err != nil {
		return rewriting.Staged{}, fmt.Errorf("read ref %s: %w", ref, err)
	}

	var nonce [8]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return rewriting.Staged{}, fmt.Errorf("name scratch ref: %w", err)
	}

	staged := rewriting.Staged{Scratch: fmt.Sprintf("refs/redigit/write/%d-%x", os.Getpid(), nonce)}

	bareDir, err := bareGitDir(ctx, repoPath)
	if err != nil {
		return rewriting.Staged{}, fmt.Errorf("stage history: %w", err)
	}

	if staged.Base, err = targetBase(string(out), ref, bareDir); err != nil {
		return rewriting.Staged{}, fmt.Errorf("stage history: %w", err)
	}

	return staged, nil
}

func bareGitDir(ctx context.Context, repoPath string) (string, error) {
	out, err := runGit(ctx, repoPath, "rev-parse", "--is-bare-repository", "--absolute-git-dir")
	if err != nil {
		return "", fmt.Errorf("read repository kind: %w", err)
	}

	isBare, dir, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	if isBare != "true" {
		return "", nil
	}

	return dir, nil
}

func targetBase(listing, ref, bareDir string) (string, error) {
	const listFields = 4

	var base string

	for line := range strings.Lines(listing) {
		fields := strings.Split(strings.TrimSuffix(line, "\n"), "\x00")
		if len(fields) != listFields {
			return "", fmt.Errorf("read ref listing: %w", errBadListing)
		}

		switch name := fields[0]; {
		case strings.HasPrefix(name, ref+"/"):
			return "", fmt.Errorf("%w: another ref lies below the target", ErrRefBusy)
		case name != ref:
		case fields[2] != "" && fields[2] != bareDir:
			// A bare repository reports its own directory as the HEAD branch's worktree.
			return "", fmt.Errorf("%w: checked out in a worktree", ErrRefBusy)
		case fields[3] != "":
			return "", fmt.Errorf("%w: a symbolic ref", ErrRefBusy)
		default:
			base = fields[1]
		}
	}

	return base, nil
}

func updateRef(ctx context.Context, repoPath, ref string, staged rewriting.Staged) error {
	if err := CheckRef(ref); err != nil {
		return fmt.Errorf("move ref: %w", err)
	}

	// An all-zero old value makes update-ref refuse a ref that exists by now.
	base := staged.Base
	if base == "" {
		base = strings.Repeat("0", len(staged.Tip))
	}

	args := []string{"update-ref", "-m", "redigit write", ref, staged.Tip, base}
	if _, err := runGitDetached(ctx, repoPath, args...); err != nil {
		return fmt.Errorf("move ref %s: %w", ref, refFailure(err))
	}

	return nil
}

func refFailure(err error) error {
	cmdErr, isCommand := errors.AsType[*commandError](err)
	if !isCommand {
		return err
	}

	tail := strings.TrimSpace(cmdErr.Stderr)

	// These are the three ways update-ref words a compare-and-swap that lost.
	for _, lost := range []string{"but expected", "already exists", "unable to resolve"} {
		if strings.Contains(tail, lost) {
			return fmt.Errorf("%w: %w: %s", ErrRefMoved, err, tail)
		}
	}

	return fmt.Errorf("%w: %s", err, tail)
}

func (h *History) Publish(ctx context.Context, ref string, staged rewriting.Staged) error {
	err := updateRef(ctx, h.repoPath, ref, staged)
	if err == nil {
		return nil
	}

	// A git killed after it committed leaves the ref moved, and that is a success.
	// The re-read is isolated and outlives a cancelled ctx for the same reason update-ref is.
	out, readErr := runGitDetached(context.WithoutCancel(ctx), h.repoPath, "rev-parse", ref)
	if readErr == nil && string(bytes.TrimSpace(out)) == staged.Tip {
		return nil
	}

	return err
}

func (h *History) Discard(ctx context.Context, staged rewriting.Staged) error {
	if _, err := runGitDetached(ctx, h.repoPath, "update-ref", "-d", staged.Scratch); err != nil {
		return fmt.Errorf("delete scratch ref: %w", refFailure(err))
	}

	return nil
}

var (
	ErrBadRef   = errors.New("bad ref name")
	ErrRefMoved = errors.New("ref moved since the write began")
	ErrRefBusy  = errors.New("ref cannot take a write")

	errBadListing = errors.New("unexpected line")
)
