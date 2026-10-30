package plancache

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/mod/modfile"

	"github.com/wrapped-owls/redigit/internal/infra/gitstore"
)

const (
	goModFile  = "go.mod"
	goWorkFile = "go.work"
)

func moduleProbeDir(ctx context.Context, repo, commit string) (dir string, err error) {
	// Only the files GOTOOLCHAIN reads, so no checkout is made to ask a version question.
	const fileMode = 0o600

	made, mkErr := os.MkdirTemp("", "redigit-toolchain-*")
	if mkErr != nil {
		return "", fmt.Errorf("%w: make a probe dir: %w", ErrToolchain, mkErr)
	}

	// Its own local: a bare-valued return blanks dir before this runs.
	defer func() {
		if err != nil {
			_ = os.RemoveAll(made)
		}
	}()

	reader := gitstore.NewReader(repo, commit)
	defer func() { err = errors.Join(err, reader.Close()) }()

	for _, name := range []string{goModFile, goWorkFile} {
		content, readErr := reader.ReadAt(ctx, commit, name)
		if errors.Is(readErr, gitstore.ErrAbsentAtRevision) {
			continue
		}

		if readErr != nil {
			return "", fmt.Errorf("%w: read the commit's %s: %w", ErrToolchain, name, readErr)
		}

		if err = refuseOffRepo(name, content); err != nil {
			return "", err
		}

		if err = os.WriteFile(filepath.Join(made, name), content, fileMode); err != nil {
			return "", fmt.Errorf("%w: write the probe %s: %w", ErrToolchain, name, err)
		}
	}

	if err = refuseNested(ctx, reader, commit); err != nil {
		return "", err
	}

	return made, nil
}

func refuseNested(ctx context.Context, reader gitstore.Reader, commit string) error {
	// The loader type-checks every module in the commit, so each go.mod is a key input.
	modes, err := reader.Modes(ctx)
	if err != nil {
		return fmt.Errorf("%w: list the commit's files: %w", ErrToolchain, err)
	}

	var nested []string

	for file := range modes {
		if path.Base(file) == goModFile && file != goModFile {
			nested = append(nested, file)
		}
	}

	// Sorted for a stable first refusal, after the filter: a big tree holds few go.mod files.
	slices.Sort(nested)

	for _, file := range nested {
		content, readErr := reader.ReadAt(ctx, commit, file)
		if readErr != nil {
			return fmt.Errorf("%w: read the commit's %s: %w", ErrToolchain, file, readErr)
		}

		if err = refuseOffRepo(file, content); err != nil {
			return err
		}
	}

	return nil
}

func refuseOffRepo(file string, content []byte) error {
	// Sources outside the repo are never in the tree, so the key could not see them change.
	var local []string

	switch path.Base(file) {
	case goModFile:
		parsed, err := modfile.Parse(file, content, nil)
		if err != nil {
			return fmt.Errorf("%w: parse the commit's %s: %w", ErrToolchain, file, err)
		}

		for _, replace := range parsed.Replace {
			if replace.New.Version == "" {
				local = append(local, replace.New.Path)
			}
		}
	case goWorkFile:
		parsed, err := modfile.ParseWork(file, content, nil)
		if err != nil {
			return fmt.Errorf("%w: parse the commit's %s: %w", ErrToolchain, file, err)
		}

		for _, use := range parsed.Use {
			local = append(local, use.Path)
		}

		for _, replace := range parsed.Replace {
			if replace.New.Version == "" {
				local = append(local, replace.New.Path)
			}
		}
	}

	if slices.ContainsFunc(local, func(target string) bool { return leavesRepo(file, target) }) {
		return fmt.Errorf("%w: the commit's %s names a path outside the repo", ErrToolchain, file)
	}

	return nil
}

func leavesRepo(file, target string) bool {
	// Windows reads \abs and C:x outside the repo, though neither is absolute to path.
	if strings.HasPrefix(target, `\`) || filepath.VolumeName(filepath.FromSlash(target)) != "" {
		return true
	}

	// A path is relative to the directory of the file that declares it.
	joined := path.Join(path.Dir(file), target)

	return filepath.IsAbs(target) || path.IsAbs(target) || !filepath.IsLocal(joined)
}
