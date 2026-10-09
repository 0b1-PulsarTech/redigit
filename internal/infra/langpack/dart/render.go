package dart

import (
	"context"
	"fmt"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// Render writes source back holding only the kept declarations, and only the
// imports they still read. An import is a compile error once its file is
// missing and a warning once nothing reads it, so a partial file keeps
// neither kind: the reference edges guarantee what it keeps exists.
func (s Syntax) Render(
	ctx context.Context,
	source []byte,
	keep []entities.DeclKey,
) ([]byte, error) {
	if len(keep) == 0 {
		return nil, nil
	}

	file := keep[0].File
	keepSet := make(map[entities.DeclKey]bool, len(keep))

	for _, key := range keep {
		keepSet[key] = true
	}

	parsed := parseFile(file, string(source))
	if keepSet[entities.DeclKey{File: file, Kind: entities.KindOther, Name: fileDeclName}] ||
		isGenerated(parsed.source) {
		return source, nil
	}

	usedImports, err := s.usedImports(ctx, parsed, keepSet)
	if err != nil {
		return nil, fmt.Errorf("render %s: %w", file, err)
	}

	return parsed.write(func(index int, it parsedItem) bool {
		switch it.kind {
		case itemImport:
			return usedImports[index]
		case itemExport, itemDecl:
			return keepSet[it.key]
		default:
			return true
		}
	}), nil
}

// usedImports reports, by item index, which imports a kept declaration reads.
// An outside library imported with neither prefix nor show list cannot be
// judged, so it stays.
func (s Syntax) usedImports(
	ctx context.Context,
	parsed parsedFile,
	keepSet map[entities.DeclKey]bool,
) (map[int]bool, error) {
	visible, err := s.space.scopeOf(ctx, parsed)
	if err != nil {
		return nil, err
	}

	through := map[int]bool{}

	for _, it := range parsed.items {
		if !keepSet[it.key] {
			continue
		}

		for _, ref := range visible.references(it.tokens(parsed.toks), it.key) {
			through[ref.through] = true
		}
	}

	out := map[int]bool{}
	importAt := 0

	for index, it := range parsed.items {
		if it.kind != itemImport {
			continue
		}

		imported := visible.imports[importAt]
		isOpaque := imported.target == "" && imported.directive.prefix == "" &&
			len(imported.directive.show) == 0
		out[index] = isOpaque || through[importAt]
		importAt++
	}

	return out, nil
}

// write tiles the file back from the items isKept accepts. The whitespace
// before an item travels with it, so dropping one never doubles a blank line;
// nothing at all is written when no declaration or export survives.
func (f parsedFile) write(isKept func(index int, it parsedItem) bool) []byte {
	if len(f.items) == 0 {
		return nil
	}

	var (
		out        strings.Builder
		prevEnd    = f.items[0].start
		hasContent bool
	)

	out.WriteString(f.source[:prevEnd])

	for index, it := range f.items {
		gap := f.source[prevEnd:it.start]
		prevEnd = it.end

		if !isKept(index, it) {
			continue
		}

		if out.Len() == 0 {
			gap = strings.TrimLeft(gap, "\r\n")
		}

		out.WriteString(gap)
		out.WriteString(f.source[it.start:it.end])
		hasContent = hasContent || it.kind == itemDecl || it.kind == itemExport
	}

	if !hasContent {
		return nil
	}

	out.WriteString(f.source[prevEnd:])

	return []byte(out.String())
}
