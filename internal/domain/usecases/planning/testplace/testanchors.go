package testplace

import (
	"maps"
	"path"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

// AnchorTests gives every test-side declaration the subject its file is about,
// so a helper, a fixture or a sentinel error is born with the code its suite
// exercises. Only a Test function names a subject on its own; the pieces around
// it carry nothing but the kernel symbols they use, and the first feature to
// freshen one of those drags them into a commit hundreds of positions before the
// file's source exists.
//
// The subject is the declarations of the twin source file; failing that, what
// the file's own tests exercise; failing that, the package. A package's other
// tests also wait for the one that opens it with TestMain (see openingTests).
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

	for index, decl := range decls {
		if label, found := opened.atoms[decl.Key]; found {
			decls[index].Atom = label
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

// raisedTests lifts each test to the position of the latest subject it waits
// for, and whatever reads a lifted test with it. The ordering was settled before
// these anchors existed, and a cut sorts by position, so a helper positioned by
// its kernel imports alone is cut ahead of the code its suite exercises, and a
// test positioned below the helper it calls is cut ahead of it.
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

// productionEdges drops the waits of production code on a test declaration. The
// method-set rule makes every implementer of an interface wait for the others, so
// a test double that satisfies apperr's port made the kernel's own error types
// wait for it, and a sentinel type of one suite was born with the kernel, a
// thousand commits before that suite. Production cannot import a test file.
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

// subjectIndex lists the production declarations a test can be about, by file
// and by directory.
type subjectIndex struct {
	order  entities.Ordering
	byFile map[string][]entities.DeclKey
	byDir  map[string][]entities.DeclKey
}

func newSubjectIndex(decls []entities.Decl, order entities.Ordering) subjectIndex {
	index := subjectIndex{
		order:  order,
		byFile: map[string][]entities.DeclKey{},
		byDir:  map[string][]entities.DeclKey{},
	}

	for _, decl := range decls {
		if entities.IsTestSide(decl) || decl.Key.Kind == entities.KindOther {
			continue
		}

		index.byFile[decl.Key.File] = append(index.byFile[decl.Key.File], decl.Key)
		index.byDir[path.Dir(decl.Key.File)] = append(
			index.byDir[path.Dir(decl.Key.File)], decl.Key,
		)
	}

	return index
}

// subjectsOf is what a test-side declaration waits for beyond what it already
// names: the twin file, else the subjects of its file's tests (for anything but
// a test with a subject of its own), else the package.
func (index subjectIndex) subjectsOf(
	decl entities.Decl,
	named []entities.DeclKey,
	fileWide map[entities.DeclKey]bool,
) []entities.DeclKey {
	if twin := index.twinOf(decl.Key.File); len(twin) > 0 {
		return index.earliest(twin)
	}

	if len(named) > 0 {
		return nil
	}

	if len(fileWide) > 0 {
		out := make([]entities.DeclKey, 0, len(fileWide))
		for key := range fileWide {
			out = append(out, key)
		}

		return index.earliest(out)
	}

	return index.earliest(index.byDir[path.Dir(decl.Key.File)])
}

// earliest keeps the subjects at the first position of the set. A helper is born
// with the first piece of the code it serves; waiting for the last, a later
// feature's method or a compile-time assertion on a port born at the end,
// carried a whole suite hundreds of commits past its source.
func (index subjectIndex) earliest(keys []entities.DeclKey) []entities.DeclKey {
	first := entities.NoPosition

	for _, key := range keys {
		if at := index.order.Of(key); first == entities.NoPosition || at < first {
			first = at
		}
	}

	var out []entities.DeclKey

	for _, key := range keys {
		if index.order.Of(key) == first {
			out = append(out, key)
		}
	}

	return out
}

// twinOf is the declarations of the source file a test file is named after: the
// longest prefix of its stem, cut at each "_", that names a file in the same
// directory.
func (index subjectIndex) twinOf(testFile string) []entities.DeclKey {
	dir, base := path.Split(testFile)
	segments := strings.Split(strings.TrimSuffix(base, "_test.go"), "_")

	for cut := len(segments); cut >= 1; cut-- {
		if keys, found := index.byFile[dir+strings.Join(segments[:cut], "_")+".go"]; found {
			return keys
		}
	}

	return nil
}
