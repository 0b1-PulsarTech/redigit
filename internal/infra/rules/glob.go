package rules

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

var (
	ErrBadGlob       = errors.New("malformed glob")
	ErrUnmatchedGlob = errors.New("glob matches nothing in the tree")
)

type pendingGlob struct {
	rule    int
	kind    string
	pattern string
	match   string
	depth   int
	prefix  string
	isDir   bool
}

func CheckGlobs(built []Rule) error {
	_, err := collectGlobs(built)

	return err
}

func Validate(built []Rule, paths []string) error {
	pending, err := collectGlobs(built)
	if err != nil {
		return err
	}

	for _, file := range paths {
		if len(pending) == 0 {
			return nil
		}

		kept := pending[:0]

		for _, glob := range pending {
			if !glob.matches(file) {
				kept = append(kept, glob)
			}
		}

		pending = kept
	}

	if len(pending) > 0 {
		first := pending[0]

		return fmt.Errorf(
			"%w: rules[%d] (%s) has %q",
			ErrUnmatchedGlob, first.rule, first.kind, first.pattern,
		)
	}

	return nil
}

func collectGlobs(built []Rule) ([]pendingGlob, error) {
	var pending []pendingGlob

	for idx, rule := range built {
		globber, found := rule.(Globber)
		if !found {
			continue
		}

		for _, pattern := range globber.Globs() {
			glob := pendingGlob{
				rule:    idx,
				kind:    rule.Type(),
				pattern: pattern,
				match:   strings.TrimSuffix(pattern, "/"),
				isDir:   strings.HasSuffix(pattern, "/"),
				depth:   -1,
			}

			if _, err := path.Match(glob.match, ""); err != nil {
				return nil, fmt.Errorf(
					"%w: rules[%d] (%s) has %q: %w", ErrBadGlob, idx, rule.Type(), pattern, err,
				)
			}

			// A path without the literal head cannot match, and the test is
			// far cheaper than path.Match.
			glob.prefix = glob.match[:strings.IndexAny(glob.match+"*", "*?[\\")]

			// Without a class a star never crosses a slash, so the matched dir
			// has exactly as many slashes as the pattern: one match, not one per ancestor.
			if !strings.Contains(glob.match, "[") {
				glob.depth = strings.Count(glob.match, "/")
			}

			pending = append(pending, glob)
		}
	}

	return pending, nil
}

func (glob pendingGlob) matches(file string) bool {
	if !strings.HasPrefix(file, glob.prefix) {
		return false
	}

	if !glob.isDir {
		ok, err := path.Match(glob.match, file)

		return err == nil && ok
	}

	if glob.depth >= 0 {
		dir, found := dirWithSlashes(file, glob.depth)

		if !found {
			return false
		}

		ok, err := path.Match(glob.match, dir)

		return err == nil && ok
	}

	// A trailing slash means any file under a directory the pattern matches.
	for dir := path.Dir(file); dir != "." && dir != "/"; dir = path.Dir(dir) {
		if ok, err := path.Match(glob.match, dir); err == nil && ok {
			return true
		}
	}

	return false
}

func dirWithSlashes(file string, slashes int) (string, bool) {
	end := -1

	for range slashes + 1 {
		next := strings.IndexByte(file[end+1:], '/')
		if next < 0 {
			return "", false
		}

		end += next + 1
	}

	return file[:end], true
}
