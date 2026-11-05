package report

import (
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

// subjectsOf maps a test declaration to the declaration it exercises, from the
// anchors a language pack produced. The link exists because nothing else
// establishes it: a test names its subject by convention, not by a reference a
// resolver can follow, and pairing on the FILE instead dates every test in it
// to whatever the file was born with.
func subjectsOf(graph entities.Graph, births indexing.Births) map[entities.DeclKey]readerDTO {
	out := map[entities.DeclKey]readerDTO{}

	for _, anchor := range graph.Anchors() {
		if anchor.Reason != entities.ReasonTestOf {
			continue
		}

		at, dated := births.At[anchor.To]
		if !dated {
			at = entities.NoPosition
		}

		unit := ""
		if subject, known := graph.Decl(anchor.To); known {
			unit = string(subject.Unit)
		}

		out[anchor.Decl] = readerDTO{
			Decl:   anchor.To.String(),
			At:     int(at),
			Reason: string(anchor.Reason),
			Unit:   unit,
		}
	}

	return out
}

// satisfyingTypes names every type that satisfies some interface, keyed by its
// unit and name so a method can be matched to its receiver even when the type
// is declared in another file of the same package.
func satisfyingTypes(graph entities.Graph) map[string]bool {
	out := map[string]bool{}

	for _, edge := range graph.Edges() {
		if edge.Reason != entities.ReasonImplements {
			continue
		}

		if decl, known := graph.Decl(edge.Dependent); known {
			out[string(decl.Unit)+"."+decl.Key.Name] = true
		}
	}

	return out
}

// inInterface reports whether a declaration is bound to an interface: the type
// itself, or a method of a type that satisfies one.
func inInterface(satisfying map[string]bool, decl entities.Decl) bool {
	name := decl.Key.Name
	if decl.Key.Kind == entities.KindMethod {
		receiver, _, found := strings.Cut(name, ".")
		if !found {
			return false
		}

		name = receiver
	}

	return satisfying[string(decl.Unit)+"."+name]
}
