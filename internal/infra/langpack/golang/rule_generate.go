package golang

import (
	"bufio"
	"bytes"
	"context"
	"path"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/declsource"
)

// Generate ties a generated file to the declaration carrying the
// "//go:generate" directive that writes it. Without it a mock was born 195
// commits before its directive - not merely early: running the generator in
// that tree finds no directive, so the file is not reproducible there.
type Generate struct{}

func (Generate) Name() string { return "go-generate" }

func (Generate) Edges(
	_ context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	byFile := declsource.GroupByFile(decls)

	var edges []entities.Edge

	for file, fileDecls := range byFile {
		source, err := declsource.ReadSource(snap, file)
		if err != nil {
			continue
		}

		for _, destination := range generateDestinations(path.Dir(file), source) {
			targets, produced := byFile[destination]
			if !produced {
				continue
			}

			// The whole file waits, not one declaration of it: the generator
			// writes every byte of the destination in one pass.
			for _, target := range targets {
				edges = append(edges, entities.Edge{
					Dependent: target.Key,
					Provider:  directiveHolder(fileDecls, target.Key),
					Reason:    entities.ReasonGeneratedBy,
				})
			}
		}
	}

	return edges, nil
}

func (Generate) Anchors(
	_ context.Context,
	_ entities.Snapshot,
	_ []entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}

// directiveHolder picks which declaration of the generating file stands for
// the directive. The directive sits above a declaration but belongs to the
// file, so the earliest-spanning one is what a caller can name; a file whose
// declarations all vanished is skipped by the caller before reaching here.
func directiveHolder(decls []entities.Decl, exclude entities.DeclKey) entities.DeclKey {
	best := decls[0].Key
	bestStart := decls[0].Span.Start

	for _, decl := range decls {
		if decl.Key == exclude {
			continue
		}

		if decl.Span.Start < bestStart {
			best, bestStart = decl.Key, decl.Span.Start
		}
	}

	return best
}

// generateDestinations reports the repo-relative files the directives in one
// source write, read from the "-destination=" flag mockgen and its kin take.
func generateDestinations(dir string, source []byte) []string {
	if !bytes.Contains(source, []byte("//go:generate")) {
		return nil
	}

	var out []string

	scanner := bufio.NewScanner(bytes.NewReader(source))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "//go:generate") {
			continue
		}

		for field := range strings.FieldsSeq(line) {
			target, named := strings.CutPrefix(field, "-destination=")
			if !named || target == "" {
				continue
			}

			out = append(out, path.Clean(path.Join(dir, target)))
		}
	}

	return out
}
