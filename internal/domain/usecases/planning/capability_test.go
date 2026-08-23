package planning

import (
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

// The whole reason this exists: a caller's slicing must survive into the
// commits. Reachability put 42% of a real repository's declarations in layer
// commits with no slice; a supplied slicing must leave none.
func TestPlannerKeepsACapabilityInItsOwnCommits(t *testing.T) {
	t.Parallel()

	graph, order, decls := twoFeatures()

	plan, err := New(Config{Capabilities: capabilitiesOf(decls)}).Plan(graph, order)
	if err != nil {
		t.Fatalf("Plan() = %v", err)
	}

	for _, commit := range plan.Commits {
		slices := map[FeatureID]bool{}

		for _, key := range commit.Decls {
			slices[capabilitiesOf(decls)[key]] = true
		}

		if len(slices) > 1 {
			t.Fatalf(
				"commit %d (%q) mixes %d capabilities: a commit must belong to one",
				commit.Position, commit.Subject, len(slices),
			)
		}
	}
}

// straddling builds the shape a caller's slicing can produce and reachability
// cannot: base(0) and top(2) are one capability, mid(1) is another. The first
// slice starts at 0 and is emitted first, and its top reads a mid no commit
// has produced yet.
func straddling() (entities.Graph, entities.Ordering, Capabilities, entities.Decl) {
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
	}, mid
}

// A declaration pulled forward to satisfy an earlier slice must not be
// emitted again when its own slice comes round: the second commit would carry
// content the first already created.
func TestPlannerEmitsEachDeclarationOnce(t *testing.T) {
	t.Parallel()

	graph, order, capabilities, _ := straddling()

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

	graph, order, capabilities, _ := straddling()

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
		for _, key := range commit.Decls {
			if key == decls["kernel"].Key {
				return
			}
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
