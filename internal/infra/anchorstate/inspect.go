package anchorstate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type conflictsKind int

const (
	conflictsAbsent conflictsKind = iota
	conflictsOurs
	conflictsForeign
)

type found struct {
	exists    bool
	state     State
	stateErr  error
	hasState  bool
	conflicts conflictsKind
	listed    []string
}

func conflictsPath(dir string) string { return filepath.Join(dir, conflictsDir) }

func inspect(dir string) (found, error) {
	stat, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return found{}, nil
	}

	if err != nil {
		return found{}, fmt.Errorf("read the state directory: %w", err)
	}

	if !stat.IsDir() {
		return found{}, fmt.Errorf("%w: %s is not a directory", ErrForeign, dir)
	}

	state, err := readState(dir)
	isOurs := errors.Is(err, ErrCorrupt) && state.Marker == marker

	if err != nil && !errors.Is(err, ErrNoState) && !isOurs {
		return found{}, err
	}

	owned := found{exists: true, state: state, hasState: err == nil || isOurs, stateErr: err}
	if errors.Is(err, ErrNoState) {
		owned.stateErr = nil
	}

	owned.conflicts, owned.listed, err = inspectConflicts(dir, owned)
	if err != nil {
		return found{}, err
	}

	return owned, nil
}

func inspectConflicts(dir string, owned found) (conflictsKind, []string, error) {
	path := conflictsPath(dir)

	stat, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return conflictsAbsent, nil, nil
	}

	if err != nil {
		return conflictsAbsent, nil, fmt.Errorf("read the conflicts directory: %w", err)
	}

	if !stat.IsDir() {
		return conflictsForeign, nil, nil
	}

	listed, hasMarker, err := readMarker(path)
	if err != nil {
		return conflictsAbsent, nil, err
	}

	// An abort that died between the marker and the directory leaves it empty beside the state.
	if hasMarker || (owned.hasState && isEmpty(path)) {
		return conflictsOurs, listed, nil
	}

	return conflictsForeign, nil, nil
}

func (owned found) refuseConflicts(dir string) error {
	path := conflictsPath(dir)

	switch owned.conflicts {
	case conflictsAbsent:
		return nil
	case conflictsForeign:
		return fmt.Errorf("%w: %s has no redigit marker", ErrForeign, path)
	}

	first, err := unlisted(path, owned.listed)
	if err != nil {
		return err
	}

	if first != "" {
		return fmt.Errorf("%w: %s is not one redigit wrote", ErrForeign, filepath.Join(path, first))
	}

	return nil
}

func isEmpty(dir string) bool {
	entries, err := os.ReadDir(dir)

	return err == nil && len(entries) == 0
}
