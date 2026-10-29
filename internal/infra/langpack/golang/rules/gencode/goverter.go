package gencode

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/declsource"
)

type Goverter struct{}

func (Goverter) Name() string { return "goverter" }

func (Goverter) Edges(
	_ context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	byFile := declsource.GroupByFile(decls)

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
			for _, decl := range generated {
				edges = append(edges, entities.Edge{
					Dependent: decl.Key,
					Provider:  provider,
					Reason:    entities.ReasonGeneratedBy,
				})
			}
		}
	}

	return edges, nil
}

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

		source, err := declsource.ReadSource(snap, file)
		if err != nil {
			return nil, err
		}

		for _, candidate := range candidates {
			if candidate.Key.Kind == entities.KindType &&
				strings.Contains(declsource.SliceSpan(source, candidate.Span), "goverter:") {
				providers = append(providers, candidate.Key)
			}
		}
	}

	return providers, nil
}

func (Goverter) Anchors(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}
