package golang

import (
	"context"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// assertionRule ties a compile-time interface assertion to the methods that
// make it hold. "var _ I = (*T)(nil)" names T and no method, so a reference
// query sees T alone - but the line compiles only once EVERY method I demands
// exists, and a method with no caller has nothing else pulling it forward.
// That failed 224 trees with "cannot use (*factEqualsRule)(nil)".
type assertionRule struct{}

func (assertionRule) Name() string { return "interface-assertion" }

func (assertionRule) Edges(
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

func (assertionRule) Anchors(
	_ context.Context,
	_ entities.Snapshot,
	_ []entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}

// receiverKey identifies a receiver inside one unit: two packages may both
// declare a type of the same name, and their methods are not interchangeable.
type receiverKey struct {
	unit entities.UnitID
	name string
}

// methodsByReceiver groups every method declaration by the type it hangs off.
// A method's key is "method:Receiver.Name", which is where the receiver comes
// from without re-reading any source.
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

// assertedType reports the concrete type a declaration asserts, by parsing its
// own text. Parsing rather than matching a name: an assertion carries type
// arguments, a pointer or a bare type, a qualified or a local interface. A
// pattern for the plain forms matched 24 of 40 real ones, and the ones it
// missed were the failing ones.
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
		read, err := ReadSource(snap, decl.Key.File)
		if err != nil {
			read = nil
		}

		source = read
		sources[decl.Key.File] = source
	}

	text := SliceSpan(source, decl.Span)
	if text == "" {
		return "", false
	}

	return receiverOfAssertion(text)
}
