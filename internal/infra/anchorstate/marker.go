package anchorstate

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func isListable(line string) bool {
	rev, name, isPair := strings.Cut(line, "/")
	if !isPair || strings.Contains(name, "/") {
		return false
	}

	isPlain := func(part string) bool {
		return filepath.IsLocal(part) && !slices.Contains([]string{".", ".."}, part)
	}

	return isPlain(rev) && isPlain(name)
}

func readMarker(root string) ([]string, bool, error) {
	path := filepath.Join(root, markerFile)

	stat, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && !stat.Mode().IsRegular()) {
		return nil, false, nil
	}

	if err != nil {
		return nil, false, fmt.Errorf("read the conflicts marker: %w", err)
	}

	// #nosec G304 -- the marker of a directory under the state directory.
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false, fmt.Errorf("read the conflicts marker: %w", err)
	}

	var listed []string

	for line := range strings.SplitSeq(string(raw), "\n") {
		if isListable(line) {
			listed = append(listed, line)
		}
	}

	return listed, true, nil
}

func writeMarker(root string, names []string) error {
	slices.Sort(names)

	body := ""
	if len(names) > 0 {
		body = strings.Join(names, "\n") + "\n"
	}

	if err := os.WriteFile(filepath.Join(root, markerFile), []byte(body), fileMode); err != nil {
		return fmt.Errorf("write the conflicts marker: %w", err)
	}

	return nil
}

func unlisted(root string, listed []string) (string, error) {
	files, revs := map[string]bool{}, map[string]bool{}

	for _, name := range listed {
		rev, _, _ := strings.Cut(name, "/")
		files[name], revs[rev] = true, true
	}

	var first string

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return fmt.Errorf("name an entry: %w", relErr)
		}

		rel = filepath.ToSlash(rel)

		isListed := rel == "." || (rel == markerFile && entry.Type().IsRegular()) ||
			(entry.IsDir() && revs[rel]) || (entry.Type().IsRegular() && files[rel])
		if isListed {
			return nil
		}

		first = rel

		return fs.SkipAll
	})
	if err != nil {
		return "", fmt.Errorf("read the conflicts directory: %w", err)
	}

	return first, nil
}

func dropTexts(root string, listed []string) error {
	var errs []error

	revs := map[string]bool{}

	for _, name := range listed {
		rev, _, _ := strings.Cut(name, "/")

		stat, err := os.Lstat(filepath.Join(root, rev))
		if err != nil || !stat.IsDir() {
			errs = append(errs, ignoreMissing(err))

			continue
		}

		revs[rev] = true

		errs = append(errs, ignoreMissing(os.Remove(filepath.Join(root, filepath.FromSlash(name)))))
	}

	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("remove a conflict text: %w", err)
	}

	for _, rev := range slices.Sorted(maps.Keys(revs)) {
		errs = append(errs, rmdirIfEmpty(filepath.Join(root, rev)))
	}

	return errors.Join(errs...)
}

func rmdirIfEmpty(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ignoreMissing(err)
	}

	if len(entries) > 0 {
		return nil
	}

	return ignoreMissing(os.Remove(dir))
}
