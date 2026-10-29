package planning

import (
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// capabilitiesOf names every declaration of twoFeatures, deliberately putting
// the kernel inside the bill slice: a caller who read the published history
// knows where a declaration was actually built, and reachability would have
// called this one shared.
func capabilitiesOf(decls map[string]entities.Decl) Capabilities {
	return Capabilities{
		decls["kernel"].Key:   "cobranca",
		decls["billUse"].Key:  "cobranca",
		decls["billAPI"].Key:  "cobranca",
		decls["leadRepo"].Key: "captacao",
		decls["leadUse"].Key:  "captacao",
		decls["leadAPI"].Key:  "captacao",
	}
}

// A commit holds one PACKAGE. Cutting on the capability instead put 31% of a
// real repository's commits at one file each: the capability changes at almost
// every commit, so consecutive declarations of one package landed apart for no
// reason a reader could see.
func TestPlannerKeepsACommitToOnePackage(t *testing.T) {
	t.Parallel()

	graph, order, decls := twoFeatures()

	plan, err := New(Config{Capabilities: capabilitiesOf(decls)}).Plan(graph, order)
	if err != nil {
		t.Fatalf("Plan() = %v", err)
	}

	for _, commit := range plan.Commits {
		dirs := map[string]bool{}
		for _, key := range commit.Decls {
			dirs[dirOf(key.File)] = true
		}

		if len(dirs) > 1 {
			t.Fatalf(
				"commit %d (%q) spans %d packages", commit.Position, commit.Subject, len(dirs),
			)
		}
	}
}

// Two declarations of one package, in different capabilities, adjacent in
// birth order: one commit, not two. This is the shape that dominated the
// single-file commits - 113 of 302 on a real repository.
func TestPlannerJoinsOnePackageAcrossCapabilities(t *testing.T) {
	t.Parallel()

	first := decl("wire/a.go", "One", "wire")
	second := decl("wire/b.go", "Two", "wire")

	graph := entities.NewGraph([]entities.Decl{first, second}, nil, nil)
	order := entities.Ordering{At: map[entities.DeclKey]entities.Position{
		first.Key: 4, second.Key: 5,
	}}

	plan, err := New(Config{Capabilities: Capabilities{
		first.Key: "chatbot", second.Key: "agenda",
	}}).Plan(graph, order)
	if err != nil {
		t.Fatalf("Plan() = %v", err)
	}

	if len(plan.Commits) != 1 {
		t.Fatalf(
			"one package became %d commits: the capability changed and the package did not",
			len(plan.Commits),
		)
	}
}

// straddling builds the shape a caller's slicing can produce and reachability
// cannot: base(0) and top(2) are one capability, mid(1) is another. The first
// slice starts at 0 and is emitted first, and its top reads a mid no commit
// has produced yet.
func straddling() (entities.Graph, entities.Ordering, Capabilities) {
	base := decl("base/id.go", "NewID", "base")
	mid := decl("mid/store.go", "Save", "mid")
	top := decl("top/handler.go", "Handle", "top")

	graph := entities.NewGraph(
		[]entities.Decl{base, mid, top},
		[]entities.Edge{needs(top, mid), needs(mid, base)},
		nil,
	)

	order := entities.Ordering{At: map[entities.DeclKey]entities.Position{
		base.Key: 0, mid.Key: 1, top.Key: 2,
	}}

	return graph, order, Capabilities{
		base.Key: "primeira",
		top.Key:  "primeira",
		mid.Key:  "segunda",
	}
}

// A declaration pulled forward to satisfy an earlier slice must not be
// emitted again when its own slice comes round: the second commit would carry
// content the first already created.
func TestPlannerEmitsEachDeclarationOnce(t *testing.T) {
	t.Parallel()

	graph, order, capabilities := straddling()

	plan, err := New(Config{Capabilities: capabilities}).Plan(graph, order)
	if err != nil {
		t.Fatalf("Plan() = %v", err)
	}

	seen := map[entities.DeclKey]int{}

	for _, commit := range plan.Commits {
		for _, key := range commit.Decls {
			seen[key]++
		}
	}

	for key, count := range seen {
		if count > 1 {
			t.Fatalf("%s appears in %d commits, want 1", key.String(), count)
		}
	}

	if len(seen) != graph.Len() {
		t.Fatalf("the plan carries %d declarations, want all %d", len(seen), graph.Len())
	}
}

// The invariant the tool exists for: with a caller's slicing, nothing is
// shared, so a slice sequenced before one it reads must still pull its
// provider forward rather than carry a reader ahead of what it reads.
func TestPlannerBornBeforeUseWithCapabilities(t *testing.T) {
	t.Parallel()

	graph, order, capabilities := straddling()

	plan, err := New(Config{Capabilities: capabilities}).Plan(graph, order)
	if err != nil {
		t.Fatalf("Plan() = %v", err)
	}

	bornAt := map[entities.DeclKey]int{}

	for _, commit := range plan.Commits {
		for _, key := range commit.Decls {
			bornAt[key] = int(commit.Position)
		}
	}

	for _, edge := range graph.Edges() {
		dependent, provider := bornAt[edge.Dependent], bornAt[edge.Provider]
		if provider > dependent {
			t.Fatalf(
				"%s is born at commit %d but needs %s, born at %d",
				edge.Dependent.String(), dependent, edge.Provider.String(), provider,
			)
		}
	}
}

// A declaration the caller's map does not name must still reach a commit: a
// slicing with a hole is allowed, and dropping what falls in it would leave a
// gap the tree invariant only catches once every explanation is gone.
func TestPlannerPlacesADeclarationNoCapabilityNames(t *testing.T) {
	t.Parallel()

	graph, order, decls := twoFeatures()

	partial := capabilitiesOf(decls)
	delete(partial, decls["kernel"].Key)

	plan, err := New(Config{Capabilities: partial}).Plan(graph, order)
	if err != nil {
		t.Fatalf("Plan() = %v", err)
	}

	for _, commit := range plan.Commits {
		if slices.Contains(commit.Decls, decls["kernel"].Key) {
			return
		}
	}

	t.Fatalf("the unnamed declaration reached no commit; unplaced = %v", plan.Unplaced)
}

// Two slices that need each other are a grouping the caller is authoritative
// about, not a reason to refuse to plan. The derived path reports that cycle;
// this one must not.
func TestPlannerDoesNotRefuseMutuallyNeedingCapabilities(t *testing.T) {
	t.Parallel()

	graph, order, decls := twoFeatures()

	// Split the lead slice across two capabilities so each needs the other.
	capabilities := Capabilities{
		decls["kernel"].Key:   "a",
		decls["leadRepo"].Key: "a",
		decls["leadUse"].Key:  "b",
		decls["leadAPI"].Key:  "a",
		decls["billUse"].Key:  "b",
		decls["billAPI"].Key:  "a",
	}

	plan, err := New(Config{Capabilities: capabilities}).Plan(graph, order)
	if err != nil {
		t.Fatalf("Plan() = %v, want a plan: the caller's grouping is authoritative", err)
	}

	if len(plan.Commits) == 0 {
		t.Fatal("Plan() returned no commit")
	}
}

// Two declarations the fixed point put at the SAME position, in different
// capabilities, one needing the other. Falling back to the key inverts a real
// dependency about half the time: this put "use ./modules/wabapi" nineteen
// commits before its go.mod, because "go.work" sorts before "modules/".
func TestPlannerOrdersATieByDependencyNotByName(t *testing.T) {
	t.Parallel()

	// The reader's key sorts BEFORE the provider's, so a name-ordered tie
	// puts it first.
	reader := decl("go.work", "use", "root")
	provider := decl("modules/wabapi/go.mod", "module", "modules/wabapi")

	graph := entities.NewGraph(
		[]entities.Decl{reader, provider},
		[]entities.Edge{needs(reader, provider)},
		nil,
	)

	order := entities.Ordering{At: map[entities.DeclKey]entities.Position{
		reader.Key:   4,
		provider.Key: 4,
	}}

	capabilities := Capabilities{reader.Key: "workspace", provider.Key: "wabapi"}

	plan, err := New(Config{Capabilities: capabilities}).Plan(graph, order)
	if err != nil {
		t.Fatalf("Plan() = %v", err)
	}

	bornAt := map[entities.DeclKey]int{}

	for _, commit := range plan.Commits {
		for _, key := range commit.Decls {
			bornAt[key] = int(commit.Position)
		}
	}

	if bornAt[reader.Key] < bornAt[provider.Key] {
		t.Fatalf(
			"the reader is born at commit %d and what it needs at %d: a tie was ordered by "+
				"name, and the name inverted the dependency",
			bornAt[reader.Key], bornAt[provider.Key],
		)
	}
}

// A topological order is not unique, and the one picked decides how many
// commits the cut produces. Among declarations nothing separates, the pass
// must keep emitting the slice it is already emitting: interleaving them costs
// a commit per switch and spreads a slice across the history.
func TestPlannerKeepsASliceTogetherAmongTies(t *testing.T) {
	t.Parallel()

	// Six declarations at one position, nothing between them, alternating
	// slices in name order. Name order gives six commits; slice-preferring
	// order gives two.
	var (
		decls = make([]entities.Decl, 0, 6)
		caps  = Capabilities{}
	)

	// The names alternate in ALPHABETICAL order, which is the shape that
	// makes name order interleave the slices.
	for index, spec := range []struct {
		file, slice string
	}{
		{"a.go", "alpha"},
		{"b.go", "beta"},
		{"c.go", "alpha"},
		{"d.go", "beta"},
		{"e.go", "alpha"},
		{"f.go", "beta"},
	} {
		one := decl(spec.file, "F", entities.UnitID(spec.file))
		decls = append(decls, one)
		caps[one.Key] = FeatureID(spec.slice)

		_ = index
	}

	at := map[entities.DeclKey]entities.Position{}
	for _, one := range decls {
		at[one.Key] = 3
	}

	plan, err := New(Config{Capabilities: caps}).Plan(
		entities.NewGraph(decls, nil, nil), entities.Ordering{At: at},
	)
	if err != nil {
		t.Fatalf("Plan() = %v", err)
	}

	if len(plan.Commits) > 2 {
		subjects := make([]string, 0, len(plan.Commits))
		for _, commit := range plan.Commits {
			subjects = append(subjects, commit.Feature)
		}

		t.Fatalf(
			"six tied declarations of two slices became %d commits (%v): the order "+
				"interleaved the slices instead of finishing one",
			len(plan.Commits), subjects,
		)
	}
}

// A commit never splits an atom. Its members were written by a tool in one
// pass, and they can carry different slices only because each was DATED
// separately - an artefact of reading births per declaration, not a reason to
// write half a generated file.
func TestPlannerNeverSplitsAnAtom(t *testing.T) {
	t.Parallel()

	const file = "store/repo_mock_test.go"

	first := entities.Decl{
		Key:  entities.DeclKey{File: file, Kind: entities.KindType, Name: "MockRepo"},
		Unit: "store",
		Atom: file,
	}
	second := entities.Decl{
		Key:  entities.DeclKey{File: file, Kind: entities.KindType, Name: "MockRepoRecorder"},
		Unit: "store",
		Atom: file,
	}

	graph := entities.NewGraph([]entities.Decl{first, second}, nil, nil)
	order := entities.Ordering{At: map[entities.DeclKey]entities.Position{
		first.Key: 7, second.Key: 7,
	}}

	// The two halves were dated into different capabilities.
	capabilities := Capabilities{first.Key: "alpha", second.Key: "beta"}

	plan, err := New(Config{Capabilities: capabilities}).Plan(graph, order)
	if err != nil {
		t.Fatalf("Plan() = %v", err)
	}

	if len(plan.Commits) != 1 {
		t.Fatalf(
			"a generated file was written across %d commits: half of it exists on its own",
			len(plan.Commits),
		)
	}
}

// A test waits for the member of its atom that needs the last feature. The
// feature freshens that member alone, and nothing is left to freshen the test:
// it is born once the last feature is built, not in the tail of what no entry
// point reached.
func TestPlannerBirthsATestWaitingOnItsAtomAfterTheLastFeature(t *testing.T) {
	t.Parallel()

	handler := decl("a/handler.go", "Handle", "a")
	counted := decl("a/count.go", "Count", "a")
	late := decl("b/handler.go", "Serve", "b")
	early := testDecl("b/early_test.go", "TestEarly", "b")
	last := testDecl("b/last_test.go", "TestLast", "b")
	early.Atom, last.Atom = "e2e-main", "e2e-main"

	graph := entities.NewGraph(
		[]entities.Decl{handler, counted, late, early, last},
		[]entities.Edge{needs(handler, counted), needs(early, counted), needs(last, late)},
		nil,
	)
	order := entities.Ordering{At: map[entities.DeclKey]entities.Position{
		handler.Key: 0, counted.Key: 1, late.Key: 2, early.Key: 3, last.Key: 4,
	}}

	plan, err := New(Config{}).Plan(graph, order)
	if err != nil {
		t.Fatalf("Plan() = %v", err)
	}

	for _, commit := range plan.Commits {
		if strings.HasPrefix(commit.Reason, "reached by no entry point") {
			t.Fatalf("commit %q holds what no feature reached", commit.Subject)
		}
	}
}
