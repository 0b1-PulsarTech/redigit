package resolve

import (
	"go/token"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/declsource"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/syntax"
)

type (
	site struct {
		file   string
		offset int
	}

	row struct {
		file string
		line int
	}

	textFile struct {
		text  []byte
		lines []int
	}

	places struct {
		snap  entities.Snapshot
		root  string
		names map[string]string
		texts map[string]textFile
	}
)

func rowOf(at entities.Location) row {
	return row{file: at.File, line: at.Line + 1}
}

func newTextFile(text []byte) textFile {
	lines := []int{0}

	for idx, char := range text {
		if char == '\n' {
			lines = append(lines, idx+1)
		}
	}

	return textFile{text: text, lines: lines}
}

func (file textFile) offset(at entities.Location) (int, bool) {
	if at.Line < 0 || at.Line >= len(file.lines) || at.Char < 0 {
		return 0, false
	}

	// A column ends at the line's text, not at its terminator, so one past it is the next line's.
	end := len(file.text)
	if at.Line+1 < len(file.lines) {
		end = file.lines[at.Line+1] - 1
		if end > file.lines[at.Line] && file.text[end-1] == '\r' {
			end--
		}
	}

	const maxBMP = 0xFFFF

	units := 0

	for pos := file.lines[at.Line]; pos <= end; {
		if units == at.Char {
			return pos, true
		}

		if pos == end || units > at.Char {
			return 0, false
		}

		char, width := utf8.DecodeRune(file.text[pos:end])
		units++

		// LSP counts UTF-16 units, so a rune outside the BMP is two.
		if char > maxBMP {
			units++
		}

		pos += width
	}

	return 0, false
}

func newPlaces(snap entities.Snapshot) *places {
	root := snap.Root
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}

	return &places{
		snap: snap, root: root, names: map[string]string{}, texts: map[string]textFile{},
	}
}

func (where *places) relative(name string) string {
	if cached, known := where.names[name]; known {
		return cached
	}

	resolved := name
	if target, err := filepath.EvalSymlinks(name); err == nil {
		resolved = target
	}

	rel, err := filepath.Rel(where.root, resolved)
	if err != nil || strings.HasPrefix(rel, "..") {
		rel = ""
	}

	where.names[name] = filepath.ToSlash(rel)

	return where.names[name]
}

func (where *places) text(rel string) (textFile, bool) {
	if cached, known := where.texts[rel]; known {
		return cached, cached.text != nil
	}

	source, err := declsource.ReadSource(where.snap, rel)
	if err != nil {
		source = nil
	}

	where.texts[rel] = newTextFile(source)

	return where.texts[rel], source != nil
}

func (where *places) at(fset *token.FileSet, pos token.Pos) (site, entities.Location, bool) {
	if !pos.IsValid() {
		return site{}, entities.Location{}, false
	}

	// A //line directive renames the file but not the offset, which stays the real file's.
	position := fset.PositionFor(pos, false)

	rel := where.relative(position.Filename)
	if rel == "" {
		return site{}, entities.Location{}, false
	}

	file, readable := where.text(rel)
	if !readable || position.Offset > len(file.text) {
		return site{}, entities.Location{}, false
	}

	return site{file: rel, offset: position.Offset},
		syntax.LocationOf(fset, rel, file.text, pos), true
}
