package anchorstate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	stepStaged  = "staged"
	stepSwapped = "swapped"
	stepWritten = "written"

	stepListed    = "listed"
	stepMarker    = "marker"
	stepConflicts = "conflicts"
	stepNamed     = "named"
)

func (store saver) step(name string) error {
	if store.hook == nil {
		return nil
	}

	return store.hook(name)
}

func (store saver) stage(dir string, state State) error {
	staging := filepath.Join(dir, stageName)

	if err := os.MkdirAll(staging, dirMode); err != nil {
		return fmt.Errorf("make the staging directory: %w", err)
	}

	var names []string

	for _, conflict := range state.Conflicts {
		names = append(names, listing(conflict)...)
	}

	// The list goes in first, so a stage that dies half written still names every file in it.
	if err := writeMarker(staging, names); err != nil {
		return err
	}

	for _, conflict := range state.Conflicts {
		if err := writeConflict(filepath.Join(staging, conflict.Rev), conflict); err != nil {
			return err
		}
	}

	return store.step(stepStaged)
}

func (store saver) swap(dir string) error {
	conflicts, old := conflictsPath(dir), filepath.Join(dir, oldName)

	if err := ignoreMissing(os.Rename(conflicts, old)); err != nil {
		return fmt.Errorf("set the old conflicts aside: %w", err)
	}

	if err := os.Rename(filepath.Join(dir, stageName), conflicts); err != nil {
		return errors.Join(
			fmt.Errorf("move the new conflicts into place: %w", err),
			ignoreMissing(os.Rename(old, conflicts)),
		)
	}

	// Only what the old marker lists goes; anything else in there is the user's and stays,
	// and the marker stays with it, so the next save refuses by name instead of deleting it.
	if err := clearTree(old); err != nil && !errors.Is(err, ErrForeign) {
		return fmt.Errorf("remove the old conflicts: %w", err)
	}

	return store.step(stepSwapped)
}

func (store saver) writeAtomic(path string, content []byte) error {
	staged, err := writeTemp(filepath.Dir(path), content)
	if err != nil {
		return err
	}

	// A kill here leaves the temporary file, which the next save or abort clears.
	if err = store.step(stepWritten); err != nil {
		return err
	}

	if err = os.Rename(staged, path); err != nil {
		return errors.Join(
			fmt.Errorf("move the file into place: %w", err), ignoreMissing(os.Remove(staged)),
		)
	}

	return nil
}
