package testplace

import (
	"maps"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

func AnchorTests(
	graph entities.Graph,
	order entities.Ordering,
) (entities.Graph, entities.Ordering) {
	decls := graph.Decls()
	index := newSubjectIndex(decls, order)
	own := reach.TestSubjects(graph)
	anchors := append([]entities.Anchor(nil), graph.Anchors()...)

	inFile := map[string]map[entities.DeclKey]bool{}

	for _, decl := range decls {
		if !entities.IsTestSide(decl) {
			continue
		}

		if inFile[decl.Key.File] == nil {
			inFile[decl.Key.File] = map[entities.DeclKey]bool{}
		}

		for _, subject := range own[decl.Key] {
			inFile[decl.Key.File][subject] = true
		}
	}

	for _, decl := range decls {
		if !entities.IsTestSide(decl) {
			continue
		}

		for _, subject := range index.subjectsOf(decl, own[decl.Key], inFile[decl.Key.File]) {
			anchors = append(anchors, entities.Anchor{
				Decl: decl.Key, To: subject, Reason: entities.ReasonTestOf,
			})
		}
	}

	opened := openingTests(decls, order)
	anchors = append(anchors, opened.anchors...)

	for slot, decl := range decls {
		if label, found := opened.atoms[decl.Key]; found {
			decls[slot].Atom = label
		}
	}

	return entities.NewGraph(
			decls,
			productionEdges(graph),
			anchors,
		), raisedTests(
			graph,
			order,
			anchors,
		)
}

func raisedTests(
	graph entities.Graph,
	order entities.Ordering,
	anchors []entities.Anchor,
) entities.Ordering {
	raised := make(map[entities.DeclKey]entities.Position, len(order.At))
	maps.Copy(raised, order.At)

	type wait struct{ dependent, provider entities.DeclKey }

	var waits []wait

	for _, anchor := range anchors {
		if anchor.Reason == entities.ReasonTestOf {
			waits = append(waits, wait{anchor.Decl, anchor.To})
		}
	}

	for _, edge := range graph.Edges() {
		if testSide(graph, edge.Dependent) {
			waits = append(waits, wait{edge.Dependent, edge.Provider})
		}
	}

	for changed := true; changed; {
		changed = false

		for _, each := range waits {
			provider, found := raised[each.provider]
			if found && provider > raised[each.dependent] {
				raised[each.dependent] = provider
				changed = true
			}
		}
	}

	order.At = raised

	return order
}

func productionEdges(graph entities.Graph) []entities.Edge {
	edges := make([]entities.Edge, 0, len(graph.Edges()))

	for _, edge := range graph.Edges() {
		if testSide(graph, edge.Provider) && !testSide(graph, edge.Dependent) {
			continue
		}

		edges = append(edges, edge)
	}

	return edges
}

func testSide(graph entities.Graph, key entities.DeclKey) bool {
	decl, found := graph.Decl(key)

	return found && entities.IsTestSide(decl)
}
