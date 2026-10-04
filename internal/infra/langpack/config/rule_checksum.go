package config

import (
	"context"
	"path"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// checksumRule makes every go.mod require wait for the go.sum beside it. The
// go command verifies each required module against that file and stops with
// "missing go.sum entry" when the hash is absent, so a require born before its
// hashes breaks every tree until they arrive.
//
// go.sum is one piece, not one per line: its entries for the transitive
// closure of a require are scattered through the file, a surplus line costs a
// build nothing, and a require pulling the whole file is the smallest rule
// that is never short of a hash.
type checksumRule struct{}

func (checksumRule) Name() string { return "checksum" }

func (checksumRule) Edges(
	_ context.Context,
	_ entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	sums := map[string][]entities.DeclKey{}

	for _, decl := range decls {
		if path.Base(decl.Key.File) == goSumFile {
			sums[path.Dir(decl.Key.File)] = append(sums[path.Dir(decl.Key.File)], decl.Key)
		}
	}

	var edges []entities.Edge

	for _, decl := range decls {
		if !strings.HasPrefix(decl.Key.Name, requirePrefix) ||
			path.Base(decl.Key.File) != goModFile {
			continue
		}

		for _, sum := range sums[path.Dir(decl.Key.File)] {
			edges = append(edges, entities.Edge{
				Dependent: decl.Key,
				Provider:  sum,
				Reason:    entities.ReasonChecksum,
			})
		}
	}

	return edges, nil
}

func (checksumRule) Anchors(
	_ context.Context,
	_ entities.Snapshot,
	_ []entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}
