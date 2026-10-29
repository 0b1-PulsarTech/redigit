package golang

import (
	"context"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/declsource"
)

// conversionRule ties a function that RETURNS an interface to the methods of
// the concrete type it returns. "func New() Rule { return &factRule{} }"
// converts implicitly, so it compiles only once factRule has every method Rule
// demands, and no reference query sees that. Same requirement as the assertion
// rule, in the form that failed 416 of 520 trees.
type conversionRule struct{}

func (conversionRule) Name() string { return "interface-conversion" }

func (conversionRule) Edges(
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

	var edges []entities.Edge

	for _, decl := range decls {
		for _, built := range convertedTypes(decl, snap, sources, interfaces) {
			for _, method := range methods[receiverKey{unit: decl.Unit, name: built}] {
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
	}

	return edges, nil
}

func (conversionRule) Anchors(
	_ context.Context,
	_ entities.Snapshot,
	_ []entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}

// interfaceNames is every interface the graph knows, by bare name. By name and
// not by unit: a result type is written qualified and the qualifier is an
// import alias this layer cannot resolve. A wrong match only ADDS a wait; a
// missed one leaves the order as it is today.
func interfaceNames(decls []entities.Decl) map[string]bool {
	out := map[string]bool{}

	for _, decl := range decls {
		if decl.IsInterface {
			out[decl.Key.Name] = true
		}
	}

	return out
}

// convertedTypes reports the local types a declaration builds and hands back
// where an interface is expected.
func convertedTypes(
	decl entities.Decl,
	snap entities.Snapshot,
	sources map[string][]byte,
	interfaces map[string]bool,
) []string {
	if decl.Key.Kind != entities.KindFunc && decl.Key.Kind != entities.KindMethod {
		return nil
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
	if text == "" || !strings.Contains(text, "return") {
		return nil
	}

	return declsource.ReturnedConcreteTypes(text, interfaces)
}
