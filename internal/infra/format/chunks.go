package format

import (
	"bytes"
	"fmt"
	"path"
	"slices"
	"strings"
	"unicode"
)

type pathSet struct {
	files map[string]struct{}
	dirs  map[string]struct{}
}

func newPathSet() pathSet {
	return pathSet{files: map[string]struct{}{}, dirs: map[string]struct{}{}}
}

func fold(name string) string {
	// No NFC (no stdlib normaliser) and no ss for an eszett, so NFD and German spellings stay apart
	return strings.Map(foldRune, name)
}

func foldRune(char rune) rune {
	// ToLower first keeps its mappings that SimpleFold lacks; a false collision only splits chunks
	char = unicode.ToLower(char)
	least := char

	for next := unicode.SimpleFold(char); next != char; next = unicode.SimpleFold(next) {
		least = min(least, next)
	}

	return unicode.ToLower(least)
}

func (set pathSet) clashes(name string) bool {
	name = fold(name)

	if _, taken := set.files[name]; taken {
		return true
	}

	if _, parent := set.dirs[name]; parent {
		return true
	}

	for dir := path.Dir(name); dir != "." && dir != "/"; dir = path.Dir(dir) {
		if _, file := set.files[dir]; file {
			return true
		}
	}

	return false
}

func (set pathSet) add(name string) {
	name = fold(name)
	set.files[name] = struct{}{}

	for dir := path.Dir(name); dir != "." && dir != "/"; dir = path.Dir(dir) {
		set.dirs[dir] = struct{}{}
	}
}

func (engine *Engine) chunks(step tool, jobs []job, budget int) [][]int {
	// the budget covers the whole command line, and every argument is quoted
	const argOverhead = 5

	var out [][]int

	fixed := 0
	for _, arg := range step.argv {
		fixed += len(arg) + argOverhead
	}

	current := make([]int, 0, engine.batchSize)

	taken, used := newPathSet(), fixed

	flush := func() {
		if len(current) > 0 {
			out = append(out, current)
		}

		current, taken, used = make([]int, 0, engine.batchSize), newPathSet(), fixed
	}

	for position, item := range jobs {
		cost := len(item.clean) + argOverhead

		// a repeated path, or a file and a directory of one name, cannot share a temp dir
		if len(current) == engine.batchSize || item.alone || taken.clashes(item.clean) ||
			len(current) > 0 && used+cost > budget {
			flush()
		}

		current, used = append(current, position), used+cost

		taken.add(item.clean)

		if item.alone {
			flush()
		}
	}

	if len(current) > 0 {
		out = append(out, current)
	}

	return out
}

func isSkipped(lang *language, content []byte) bool {
	const markerWindow = 400

	return len(lang.marker) > 0 &&
		bytes.Contains(content[:min(len(content), markerWindow)], lang.marker)
}

func checkIsolate(pattern, name string) error {
	if strings.Contains(pattern, "/") {
		return fmt.Errorf("%w: isolate pattern %q of %q holds a slash", ErrConfig, pattern, name)
	}

	if _, err := path.Match(pattern, ""); err != nil {
		return fmt.Errorf("%w: isolate pattern %q of %q: %w", ErrConfig, pattern, name, err)
	}

	return nil
}

func (engine *Engine) isolated(lang *language, clean string) bool {
	if _, copied := engine.copyNames[fold(clean)]; copied {
		return true
	}

	// A config blob never shares a directory with others, since a formatter may run a config it
	// finds. A pattern matches the raw base name, and folded against the folded name, so a class
	// is never folded on its own but a case-only difference still isolates.
	base := path.Base(clean)
	folded := fold(base)

	return slices.ContainsFunc(lang.isolate, func(pattern string) bool {
		raw, _ := path.Match(pattern, base)
		blind, _ := path.Match(fold(pattern), folded)

		return raw || blind
	})
}
