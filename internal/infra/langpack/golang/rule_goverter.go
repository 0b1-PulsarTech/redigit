package golang

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// goverterRule links a generated converter_gen.go to the sibling *conv.go
// interface, carrying a "goverter:" directive, that goverter read to produce
// it. Neither file names the other: the link exists only because both live
// in the same directory and one runs a generator whose directive lives in
// the other.
type goverterRule struct{}

func (goverterRule) Name() string { return "goverter" }

func (goverterRule) Edges(
	_ context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	byFile := groupByFile(decls)

	var edges []entities.Edge

	for file, generated := range byFile {
		if filepath.Base(file) != "converter_gen.go" {
			continue
		}

		providers, err := directiveDecls(snap, byFile, filepath.Dir(file))
		if err != nil {
			return nil, err
		}

		for _, provider := range providers {
			for _, g := range generated {
				edges = append(edges, entities.Edge{
					Dependent: g.Key,
					Provider:  provider,
					Reason:    entities.ReasonGeneratedBy,
				})
			}
		}
	}

	return edges, nil
}

// directiveDecls returns the type declarations in dir's *conv.go siblings
// whose own span carries a "goverter:" directive.
func directiveDecls(
	snap entities.Snapshot,
	byFile map[string][]entities.Decl,
	dir string,
) ([]entities.DeclKey, error) {
	var providers []entities.DeclKey

	for file, candidates := range byFile {
		if filepath.Dir(file) != dir || !strings.HasSuffix(file, "conv.go") {
			continue
		}

		source, err := readSource(snap, file)
		if err != nil {
			return nil, err
		}

		for _, d := range candidates {
			if d.Key.Kind == entities.KindType &&
				strings.Contains(sliceSpan(source, d.Span), "goverter:") {
				providers = append(providers, d.Key)
			}
		}
	}

	return providers, nil
}

func (goverterRule) Anchors(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}
