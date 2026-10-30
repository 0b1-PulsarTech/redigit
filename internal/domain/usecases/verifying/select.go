package verifying

import (
	"fmt"
	"path"
	"strings"
)

func isExcluded(dir string, prefixes []string) bool {
	dir = path.Clean(dir)

	for _, raw := range prefixes {
		prefix := path.Clean(raw)
		if prefix == "." {
			continue
		}

		if dir == prefix || strings.HasPrefix(dir, prefix+"/") {
			return true
		}
	}

	return false
}

func (opts Options) validate() error {
	switch {
	case opts.Stride < 0, opts.Parallel < 0, opts.From < 0, opts.To < 0:
		return fmt.Errorf("%w: a negative value", ErrInvalid)
	case opts.To > 0 && opts.From > opts.To:
		return fmt.Errorf("%w: from is after to", ErrInvalid)
	}

	return nil
}

func Select(commits []Commit, opts Options) ([]Commit, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}

	if opts.From > len(commits) {
		return nil, fmt.Errorf("%w: from is past the last commit", ErrInvalid)
	}

	first := max(opts.From, 1)
	last := len(commits)

	if opts.To > 0 {
		last = min(opts.To, last)
	}

	stride := max(opts.Stride, 1)
	picked := []Commit{}

	for pos := first; pos <= last; pos += stride {
		picked = append(picked, commits[pos-1])
	}

	// The tip of the range is always checked, whatever the stride skipped.
	if last >= first && (last-first)%stride != 0 {
		picked = append(picked, commits[last-1])
	}

	return picked, nil
}
