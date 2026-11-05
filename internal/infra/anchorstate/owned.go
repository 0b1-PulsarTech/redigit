package anchorstate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func isTemp(name string) bool {
	digits, hasPrefix := strings.CutPrefix(name, tmpPrefix)

	return hasPrefix && digits != "" && strings.Trim(digits, "0123456789") == ""
}

func reservedIn(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, ignoreMissing(err)
	}

	var names []string

	for _, entry := range entries {
		name := entry.Name()
		if name == stageName || name == oldName || isTemp(name) {
			names = append(names, name)
		}
	}

	return names, nil
}

func clearOwned(dir string, names []string) error {
	var failed, foreign []error

	for _, name := range names {
		err := clearTree(filepath.Join(dir, name))

		switch {
		case err == nil:
		case errors.Is(err, ErrForeign):
			foreign = append(foreign, err)
		default:
			failed = append(failed, err)
		}
	}

	// ErrForeign out of here means a file of the user's and nothing worse.
	if len(failed) > 0 {
		return errors.Join(failed...)
	}

	return errors.Join(foreign...)
}

func wrapRemove(err error) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("remove an owned entry: %w", err)
}

func clearTree(path string) error {
	stat, err := os.Lstat(path)
	if err != nil {
		return ignoreMissing(err)
	}

	if !stat.IsDir() {
		return wrapRemove(os.Remove(path))
	}

	listed, hasMarker, err := readMarker(path)
	if err != nil {
		return err
	}

	if hasMarker {
		if err = dropTexts(path, listed); err != nil {
			return err
		}

		if err = dropMarkerAlone(path); err != nil {
			return err
		}
	}

	if err = rmdirIfEmpty(path); err != nil {
		return err
	}

	if _, err = os.Lstat(path); err == nil {
		return fmt.Errorf("%w: %s still holds what redigit did not write", ErrForeign, path)
	}

	return nil
}

func dropMarkerAlone(root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return ignoreMissing(err)
	}

	// The marker goes last and only alone, so what it lists survives a file that is not ours.
	if len(entries) != 1 || entries[0].Name() != markerFile {
		return nil
	}

	return ignoreMissing(os.Remove(filepath.Join(root, markerFile)))
}
