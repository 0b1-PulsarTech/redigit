package wiring

import (
	"context"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/declsource"
)

type Assertion struct{}

func (Assertion) Name() string { return "interface-assertion" }

func (Assertion) Edges(
	_ context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	methods := methodsByReceiver(decls)
	sources := map[string][]byte{}

	var edges []entities.Edge

	for _, decl := range decls {
		receiver, asserts := assertedType(decl, snap, sources)
		if !asserts {
			continue
		}

		for _, method := range methods[receiverKey{unit: decl.Unit, name: receiver}] {
			if method == decl.Key {
				continue
			}

			edges = append(edges, entities.Edge{
				Dependent: decl.Key,
				Provider:  method,
				Reason:    entities.ReasonImplements,
			})
		}
	}

	return edges, nil
}

func (Assertion) Anchors(
	_ context.Context,
	_ entities.Snapshot,
	_ []entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}

type receiverKey struct {
	unit entities.UnitID
	name string
}

func methodsByReceiver(decls []entities.Decl) map[receiverKey][]entities.DeclKey {
	out := map[receiverKey][]entities.DeclKey{}

	for _, decl := range decls {
		if decl.Key.Kind != entities.KindMethod {
			continue
		}

		receiver, _, found := strings.Cut(decl.Key.Name, ".")
		if !found {
			continue
		}

		key := receiverKey{unit: decl.Unit, name: receiver}
		out[key] = append(out[key], decl.Key)
	}

	return out
}

func assertedType(
	decl entities.Decl,
	snap entities.Snapshot,
	sources map[string][]byte,
) (string, bool) {
	if decl.Key.Kind != entities.KindValue || !strings.HasPrefix(decl.Key.Name, "_ ") {
		return "", false
	}

	source, cached := sources[decl.Key.File]
	if !cached {
		read, err := declsource.ReadSource(snap, decl.Key.File)
		if err != nil {
			read = nil
		}

		source = read
		sources[decl.Key.File] = source
	}

	text := declsource.SliceSpan(source, decl.Span)
	if text == "" {
		return "", false
	}

	return receiverOfAssertion(text)
}
