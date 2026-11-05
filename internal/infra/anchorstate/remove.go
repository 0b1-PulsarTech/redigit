package anchorstate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func Remove(dir string) error { return saver{}.remove(dir) }

func (store saver) remove(dir string) error {
	owned, err := inspect(dir)
	if err != nil {
		return err
	}

	names, err := reservedIn(dir)
	if err != nil {
		return fmt.Errorf("read the state directory: %w", err)
	}

	if _, statErr := os.Lstat(filepath.Join(dir, DerivedFile)); statErr == nil {
		names = append(names, DerivedFile)
	}

	if !owned.hasState && owned.conflicts != conflictsOurs && len(names) == 0 {
		return ErrNoState
	}

	// Everything a retry needs stays until the last step: state.json goes after all of it.
	err = store.dropConflicts(dir, owned)
	if err == nil {
		err = clearOwned(dir, names)
	}

	if err == nil || errors.Is(err, ErrForeign) {
		err = store.step(stepNamed)
	}

	if err == nil && owned.hasState {
		err = ignoreMissing(os.Remove(filepath.Join(dir, StateFile)))
	}

	if err != nil {
		return fmt.Errorf("remove the stop state: %w", err)
	}

	if owned.state.Created {
		// Fails, and is meant to, while the user keeps anything else in the directory.
		_ = os.Remove(dir)
	}

	return nil
}

func (store saver) dropConflicts(dir string, owned found) error {
	if owned.conflicts != conflictsOurs {
		return nil
	}

	path := conflictsPath(dir)
	markerPath := filepath.Join(path, markerFile)

	steps := []struct {
		name string
		act  func() error
	}{
		{name: stepListed, act: func() error { return dropTexts(path, owned.listed) }},
		{name: stepMarker, act: func() error { return ignoreMissing(os.Remove(markerPath)) }},
		{name: stepConflicts, act: func() error { return rmdirIfEmpty(path) }},
	}

	for _, step := range steps {
		if err := step.act(); err != nil {
			return err
		}

		if err := store.step(step.name); err != nil {
			return err
		}
	}

	return nil
}
