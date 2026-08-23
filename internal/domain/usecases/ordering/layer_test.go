package ordering

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// span builds the inclusive run of positions from first to last.
func span(first, last entities.Position) []entities.Position {
	at := make([]entities.Position, 0, last-first+1)
	for position := first; position <= last; position++ {
		at = append(at, position)
	}

	return at
}

func layerDecl(file, name string) entities.Decl {
	return entities.Decl{
		Key:  entities.DeclKey{File: file, Kind: entities.KindFunc, Name: name},
		Unit: entities.UnitID(file),
	}
}

// touchAt puts one path in the history at one position, which is what seed
// reads to give a declaration its base.
func touchAt(at entities.Position, paths ...string) entities.SourceTouch {
	return entities.SourceTouch{Position: at, Rev: "r", Touched: paths}
}

func kernelLayers() entities.Layers {
	return entities.NewLayers([]entities.LayerRule{
		{Prefix: "libs/tereckernel", Layer: 1},
		{Prefix: "libs/engines", Layer: 2},
		{Prefix: "libs/infra", Layer: 6},
	})
}

// The whole point of the parameter: a kernel the published history wrote late
// is pulled forward to sit no later than the earliest thing above it.
func TestApplyLayersPullsALateKernelForward(t *testing.T) {
	t.Parallel()

	decls := []entities.Decl{
		layerDecl("libs/tereckernel/lead.go", "Lead"),
		layerDecl("libs/engines/flow.go", "Flow"),
	}

	base := map[entities.DeclKey]entities.Position{
		decls[0].Key: 400,
		decls[1].Key: 30,
	}

	applyLayers(decls, base, kernelLayers(), entities.Features{})

	if got := base[decls[0].Key]; got != 30 {
		t.Fatalf("kernel base = %d, want 30 (the earliest seed above it)", got)
	}

	if got := base[decls[1].Key]; got != 30 {
		t.Fatalf("engines base = %d, want 30 (nothing above it moved it)", got)
	}
}

// A layer only ever LOWERS a seed. Raising one could push a declaration past
// something that needs it, and no later pass would catch that.
func TestApplyLayersNeverRaisesASeed(t *testing.T) {
	t.Parallel()

	decls := []entities.Decl{
		layerDecl("libs/tereckernel/lead.go", "Lead"),
		layerDecl("libs/infra/repo.go", "Repo"),
	}

	base := map[entities.DeclKey]entities.Position{
		decls[0].Key: 5,
		decls[1].Key: 900,
	}

	applyLayers(decls, base, kernelLayers(), entities.Features{})

	testCases := []struct {
		name string
		key  entities.DeclKey
		want entities.Position
	}{
		{name: "kernel already early enough", key: decls[0].Key, want: 5},
		{name: "infra has nothing above it", key: decls[1].Key, want: 900},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := base[testCase.key]; got != testCase.want {
				t.Fatalf("base = %d, want %d", got, testCase.want)
			}
		})
	}
}

func TestApplyLayersLeavesAnUnmatchedPathAlone(t *testing.T) {
	t.Parallel()

	decls := []entities.Decl{
		layerDecl("libs/tereckernel/lead.go", "Lead"),
		layerDecl("scripts/gen.go", "Gen"),
	}

	base := map[entities.DeclKey]entities.Position{decls[0].Key: 400, decls[1].Key: 77}

	applyLayers(decls, base, kernelLayers(), entities.Features{})

	if got := base[decls[1].Key]; got != 77 {
		t.Fatalf("unmatched path base = %d, want 77 unchanged", got)
	}
}

func TestApplyLayersWithNoRulesChangesNothing(t *testing.T) {
	t.Parallel()

	decls := []entities.Decl{layerDecl("libs/tereckernel/lead.go", "Lead")}
	base := map[entities.DeclKey]entities.Position{decls[0].Key: 400}

	applyLayers(decls, base, entities.Layers{}, entities.Features{})

	if got := base[decls[0].Key]; got != 400 {
		t.Fatalf("base = %d, want 400 unchanged", got)
	}
}

// The property the whole design rests on: a layer is a tiebreak, so it must
// never place a declaration before something it depends on. Here the kernel
// declaration NEEDS an infra one, which the layers would rather have come
// last. The graph has to win.
func TestLayersNeverOverrideADependency(t *testing.T) {
	t.Parallel()

	kernel := layerDecl("libs/tereckernel/lead.go", "Lead")
	infra := layerDecl("libs/infra/repo.go", "Repo")

	graph := entities.NewGraph(
		[]entities.Decl{kernel, infra},
		[]entities.Edge{{
			Dependent: kernel.Key,
			Provider:  infra.Key,
			Reason:    entities.ReasonReference,
		}},
		nil,
	)

	snap := entities.Snapshot{
		Files: []string{kernel.Key.File, infra.Key.File},
		History: []entities.SourceTouch{
			touchAt(0, infra.Key.File),
			touchAt(9, kernel.Key.File),
		},
	}

	order, err := New(Config{Layers: kernelLayers()}).Order(graph, snap)
	if err != nil {
		t.Fatalf("Order() = %v", err)
	}

	kernelAt, infraAt := order.Of(kernel.Key), order.Of(infra.Key)
	if kernelAt < infraAt {
		t.Fatalf(
			"kernel landed at %d and the infra it needs at %d: the layer overrode the graph",
			kernelAt, infraAt,
		)
	}
}

// With no dependency between them, the layer decides: the kernel comes first
// even though the published history wrote it hundreds of positions later.
func TestLayersDecideWhenTheGraphIsIndifferent(t *testing.T) {
	t.Parallel()

	kernel := layerDecl("libs/tereckernel/lead.go", "Lead")
	infra := layerDecl("libs/infra/repo.go", "Repo")

	graph := entities.NewGraph([]entities.Decl{kernel, infra}, nil, nil)

	snap := entities.Snapshot{
		Files: []string{kernel.Key.File, infra.Key.File},
		History: []entities.SourceTouch{
			touchAt(0, infra.Key.File),
			touchAt(9, kernel.Key.File),
		},
	}

	withLayers, err := New(Config{Layers: kernelLayers()}).Order(graph, snap)
	if err != nil {
		t.Fatalf("Order() with layers = %v", err)
	}

	if got := withLayers.Of(kernel.Key); got != 0 {
		t.Fatalf("kernel landed at %d, want 0: nothing stopped the layer moving it", got)
	}

	// Without the parameter the published history stands, which is what makes
	// this a parameter rather than a change of policy.
	plain, err := New(Config{}).Order(graph, snap)
	if err != nil {
		t.Fatalf("Order() without layers = %v", err)
	}

	if got := plain.Of(kernel.Key); got != 9 {
		t.Fatalf("kernel landed at %d without layers, want the history's 9", got)
	}
}

// An engine belonging to a LATE feature must not be dragged back by a module
// belonging to an EARLY one. The positions are real ones from terectek_comms:
// schedagenda is born at 243, one commit before the falatuxat module that
// consumes it, because the two are the same feature.
func TestALayerDoesNotReachAcrossFeatures(t *testing.T) {
	t.Parallel()

	earlyModule := layerDecl("modules/wabapi/send.go", "Send")
	agendaEngine := layerDecl("libs/engines/schedagenda.go", "Slot")
	agendaModule := layerDecl("modules/falatuxat/appointment.go", "Appointment")

	decls := []entities.Decl{earlyModule, agendaEngine, agendaModule}

	base := map[entities.DeclKey]entities.Position{
		earlyModule.Key:  40,
		agendaEngine.Key: 243,
		agendaModule.Key: 244,
	}

	layers := entities.NewLayers([]entities.LayerRule{
		{Prefix: "libs/engines", Layer: 2},
		{Prefix: "modules", Layer: 5},
	})

	features := entities.NewFeatures([]entities.Feature{
		{Name: "wabapi", At: span(0, 100)},
		{Name: "agendamento", At: span(243, 245)},
	})

	applyLayers(decls, base, layers, features)

	// Reaching across features pulls the agenda engine back to 40, to sit
	// before a module of a feature it has nothing to do with.
	if got := base[agendaEngine.Key]; got != 243 {
		t.Fatalf(
			"the agenda engine moved to %d: a layer reached out of its own feature and "+
				"pulled it back to a module of another one",
			got,
		)
	}

	if got := base[earlyModule.Key]; got != 40 {
		t.Fatalf("the early module moved to %d, want 40", got)
	}
}

// Inside one feature the layer still does its job: the engine of that feature
// is seeded no later than the module that consumes it.
func TestALayerStillOrdersInsideItsOwnFeature(t *testing.T) {
	t.Parallel()

	engine := layerDecl("libs/engines/schedagenda.go", "Slot")
	module := layerDecl("modules/falatuxat/appointment.go", "Appointment")

	decls := []entities.Decl{engine, module}

	// The history happens to have written the module first.
	base := map[entities.DeclKey]entities.Position{engine.Key: 245, module.Key: 243}

	layers := entities.NewLayers([]entities.LayerRule{
		{Prefix: "libs/engines", Layer: 2},
		{Prefix: "modules", Layer: 5},
	})

	features := entities.NewFeatures([]entities.Feature{{Name: "agendamento", At: span(243, 245)}})

	applyLayers(decls, base, layers, features)

	if got := base[engine.Key]; got != 243 {
		t.Fatalf("engine seed = %d, want 243 (no later than the module of its own feature)", got)
	}
}

// A declaration whose seed falls in no feature has no slice to be ordered
// inside of, so the layer leaves it where the history put it.
func TestALayerSkipsADeclarationNoFeatureCovers(t *testing.T) {
	t.Parallel()

	orphan := layerDecl("libs/tereckernel/lead.go", "Lead")
	inside := layerDecl("libs/engines/flow.go", "Flow")

	decls := []entities.Decl{orphan, inside}
	base := map[entities.DeclKey]entities.Position{orphan.Key: 700, inside.Key: 10}

	features := entities.NewFeatures([]entities.Feature{{Name: "early", At: span(0, 100)}})

	applyLayers(decls, base, kernelLayers(), features)

	if got := base[orphan.Key]; got != 700 {
		t.Fatalf("a declaration outside every feature moved to %d, want 700", got)
	}
}
