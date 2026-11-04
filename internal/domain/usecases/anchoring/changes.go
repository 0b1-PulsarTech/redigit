package anchoring

import (
	"context"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/lineage"
)

var errUnparsed = errors.New("blob does not parse")

type (
	changed struct {
		atAnchor entities.DeclKey
		preimage string
	}

	sliceKey struct{ file, name string }

	declName struct{ kind, name string }
)

func (planner Planner) changedDecls(
	ctx context.Context,
	parser lineage.Parser,
	change FileChange,
) ([]changed, []entities.DeclKey, error) {
	var (
		oldSrc   []byte
		oldDecls []entities.Decl
		err      error
	)

	if change.OldBlob != "" {
		oldSrc, oldDecls, err = planner.parsed(ctx, parser, change.OldPath, change.OldBlob)
		if err != nil {
			return nil, nil, fmt.Errorf("parent revision: %w", err)
		}
	}

	newSrc, newDecls, err := planner.parsed(ctx, parser, change.Path, change.Blob)
	if err != nil {
		return nil, nil, err
	}

	before := make(map[declName]string, len(oldDecls))

	for _, decl := range oldDecls {
		before[declName{string(decl.Key.Kind), decl.Key.Name}] = textOf(oldSrc, decl)
	}

	var (
		out  []changed
		born []entities.DeclKey
	)

	for _, decl := range newDecls {
		old, isKnown := before[declName{string(decl.Key.Kind), decl.Key.Name}]
		if !isKnown {
			born = append(born, decl.Key)
		}

		// Moved whitespace renders to the same file, so it would make an empty commit.
		if isKnown && squeezed(old) != squeezed(textOf(newSrc, decl)) {
			out = append(out, changed{atAnchor: decl.Key, preimage: old})
		}
	}

	return out, born, nil
}

func (planner Planner) parsed(
	ctx context.Context,
	parser lineage.Parser,
	path, oid string,
) ([]byte, []entities.Decl, error) {
	src, err := planner.cfg.Revisions.Blob(ctx, oid)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}

	decls, err := parser.Declarations(ctx, path, src)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: parse %s: %w", errUnparsed, path, err)
	}

	return src, decls, nil
}

func textOf(src []byte, decl entities.Decl) string {
	return string(src[decl.Span.Start:decl.Span.End])
}

func (planner Planner) languageFor(path string) (lineage.Language, bool) {
	for _, language := range planner.cfg.Languages {
		for _, ext := range language.Extensions {
			if strings.HasSuffix(path, ext) {
				return language, true
			}
		}
	}

	return lineage.Language{}, false
}

func slicedFuncs(graph entities.Graph) map[sliceKey]bool {
	sliced := map[sliceKey]bool{}

	for _, decl := range graph.Decls() {
		if decl.Key.Kind == entities.KindStmt {
			name, _, _ := strings.Cut(decl.Key.Name, "#")
			sliced[sliceKey{decl.Key.File, name}] = true
		}
	}

	return sliced
}

func squeezed(text string) string {
	return strings.Join(strings.Fields(text), "")
}

func wordIn(text, name string) bool {
	// RE2's \b is ASCII-only: it never matches beside a letter like \u00c9, so a name would hide.
	const outside = `[^\p{L}\p{N}_]`

	return strings.Contains(text, name) &&
		regexp.MustCompile(`(?:^|`+outside+`)`+regexp.QuoteMeta(name)+`(?:$|`+outside+`)`).
			MatchString(text)
}

func (view scene) readsAfter(
	placed entry,
	index map[entities.DeclKey]int,
	moving map[entities.DeclKey]bool,
	activation int,
) string {
	if placed.key.Kind == entities.KindOther {
		return ""
	}

	home, _ := view.graph.Decl(placed.key)

	for _, decl := range view.decls {
		at, isPlaced := index[decl.Key]
		if !isPlaced || moving[decl.Key] || at <= activation || decl.Key == placed.key {
			continue
		}

		if entities.IsTestSide(decl) && !entities.IsTestSide(home) {
			continue
		}

		if !slices.Contains(
			[]entities.DeclKind{entities.KindFunc, entities.KindType, entities.KindValue},
			decl.Key.Kind,
		) {
			continue
		}

		name := decl.Key.Name
		if decl.Unit != home.Unit {
			name = path.Base(decl.Unit.Dir()) + "." + name
		}

		if wordIn(placed.preimage, decl.Key.Name) && view.reads(placed, name) {
			return decl.Key.String()
		}
	}

	return ""
}
