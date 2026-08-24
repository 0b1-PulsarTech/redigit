package golang

import (
	"context"
	"go/ast"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// boundRule ties every declaration that MENTIONS an interface-bound concrete
// type to that type's methods; boundTypes says what binds one. Enumerating the
// syntaxes that hand T to an interface does not converge - four rounds found
// four - because the right question needs a type checker this layer lacks.
type boundRule struct{}

func (boundRule) Name() string { return "interface-bound" }

func (boundRule) Edges(
	_ context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	interfaces := interfaceNames(decls)
	if len(interfaces) == 0 {
		return nil, nil
	}

	methods := methodsByReceiver(decls)
	sources := map[string][]byte{}
	bound := boundTypes(decls, snap, sources, interfaces, methods)

	if len(bound) == 0 {
		return nil, nil
	}

	var edges []entities.Edge

	for _, decl := range decls {
		for _, name := range mentionedTypes(decl, snap, sources) {
			key := receiverKey{unit: decl.Unit, name: name}

			// A method of T mentions T in its own receiver, so waiting here
			// would make every method of T wait for every other one: a clique,
			// which is a cycle, and one that crosses files whenever the
			// methods do. The methods are what the wait is FOR; they cannot
			// wait for themselves.
			if !bound[key] || isMethodOf(decl.Key, name) {
				continue
			}

			for _, method := range methods[key] {
				if method != decl.Key {
					edges = append(edges, entities.Edge{
						Dependent: decl.Key,
						Provider:  method,
						Reason:    entities.ReasonImplements,
					})
				}
			}
		}
	}

	return edges, nil
}

func (boundRule) Anchors(
	_ context.Context,
	_ entities.Snapshot,
	_ []entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}

// boundTypes are the concrete types something in their own package hands to an
// interface, which the assertion and return forms detect reliably. Every
// declaration naming one then waits for its whole method set: coarse, but
// delaying is the safe direction. A type with no method is never bound.
func boundTypes(
	decls []entities.Decl,
	snap entities.Snapshot,
	sources map[string][]byte,
	interfaces map[string]bool,
	methods map[receiverKey][]entities.DeclKey,
) map[receiverKey]bool {
	out := map[receiverKey]bool{}

	mark := func(unit entities.UnitID, name string) {
		key := receiverKey{unit: unit, name: name}
		if len(methods[key]) > 0 {
			out[key] = true
		}
	}

	for _, decl := range decls {
		if name, asserts := assertedType(decl, snap, sources); asserts {
			mark(decl.Unit, name)
		}

		for _, name := range convertedTypes(decl, snap, sources, interfaces) {
			mark(decl.Unit, name)
		}
	}

	return out
}

// mentionedTypes reports the identifiers a declaration names, from its parsed
// text rather than its bytes: a name inside a comment or a string is not a
// mention, and matching text would tie a wait to prose.
func mentionedTypes(
	decl entities.Decl,
	snap entities.Snapshot,
	sources map[string][]byte,
) []string {
	source, cached := sources[decl.Key.File]
	if !cached {
		read, err := readSource(snap, decl.Key.File)
		if err != nil {
			read = nil
		}

		source = read
		sources[decl.Key.File] = source
	}

	text := sliceSpan(source, decl.Span)
	if text == "" {
		return nil
	}

	node := parseDeclaration(text)
	if node == nil {
		return nil
	}

	seen := map[string]bool{}

	var out []string

	ast.Inspect(node, func(n ast.Node) bool {
		if ident, is := n.(*ast.Ident); is && !seen[ident.Name] {
			seen[ident.Name] = true

			out = append(out, ident.Name)
		}

		return true
	})

	return out
}

// isMethodOf reports whether key is a method hanging off the named receiver.
func isMethodOf(key entities.DeclKey, receiver string) bool {
	if key.Kind != entities.KindMethod {
		return false
	}

	name, _, found := strings.Cut(key.Name, ".")

	return found && name == receiver
}
