package pinrule

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/ordering"
)

func declOf(file, name string) entities.Decl {
	return entities.Decl{Key: entities.DeclKey{File: file, Kind: entities.KindFunc, Name: name}}
}

func stmtOf(file, name string) entities.Decl {
	return entities.Decl{Key: entities.DeclKey{File: file, Kind: entities.KindStmt, Name: name}}
}

func packOf(t *testing.T, configs []string) Pack {
	t.Helper()

	pins := make([]*Rule, 0, len(configs))

	for _, raw := range configs {
		built, err := Constructor("pin")([]byte(raw))
		if err != nil {
			t.Fatalf("Constructor(%s) = %v", raw, err)
		}

		pin, isPin := built.(*Rule)
		if !isPin {
			t.Fatalf("Constructor() built %T, want *Rule", built)
		}

		pins = append(pins, pin)
	}

	return NewPack(pins...)
}

func TestPackEdges(t *testing.T) {
	t.Parallel()

	decls := []entities.Decl{
		declOf("a.go", "A"),
		declOf("a_test.go", "TestA"),
		declOf("a_test.go", "TestB"),
		declOf("b.go", "B"),
		declOf("c.go", "C"),
	}
	keyA, keyTestA, keyTestB := decls[0].Key, decls[1].Key, decls[2].Key
	keyB, keyC := decls[3].Key, decls[4].Key

	testCases := []struct {
		name      string
		configs   []string
		wantEdges []entities.Edge
	}{
		{name: "no pin is no edge"},
		{
			name:    "key to key adds one edge from the pinned to what it follows",
			configs: []string{`{"type":"pin","decl":"a.go:func:A","after":"b.go:func:B"}`},
			wantEdges: []entities.Edge{
				{Dependent: keyA, Provider: keyB, Reason: entities.ReasonPin},
			},
		},
		{
			name:    "a glob selects by file and symbol",
			configs: []string{`{"type":"pin","decl":"*_test.go:func:Test*","after":"c.go:*:C"}`},
			wantEdges: []entities.Edge{
				{Dependent: keyTestA, Provider: keyC, Reason: entities.ReasonPin},
				{Dependent: keyTestB, Provider: keyC, Reason: entities.ReasonPin},
			},
		},
		{
			name: "two pins add both edges",
			configs: []string{
				`{"type":"pin","decl":"a.go:func:A","after":"b.go:func:B"}`,
				`{"type":"pin","decl":"b.go:func:B","after":"c.go:func:C"}`,
			},
			wantEdges: []entities.Edge{
				{Dependent: keyA, Provider: keyB, Reason: entities.ReasonPin},
				{Dependent: keyB, Provider: keyC, Reason: entities.ReasonPin},
			},
		},
		{
			name:    "a test after a production decl is allowed",
			configs: []string{`{"type":"pin","decl":"a_test.go:func:TestA","after":"a.go:func:A"}`},
			wantEdges: []entities.Edge{
				{Dependent: keyTestA, Provider: keyA, Reason: entities.ReasonPin},
			},
		},
		{
			name: "a test after a test is allowed",
			configs: []string{
				`{"type":"pin","decl":"a_test.go:func:TestA","after":"a_test.go:func:TestB"}`,
			},
			wantEdges: []entities.Edge{
				{Dependent: keyTestA, Provider: keyTestB, Reason: entities.ReasonPin},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := packOf(t, testCase.configs).
				Edges(context.Background(), entities.Snapshot{}, decls)
			if err != nil {
				t.Fatalf("Edges() = %v", err)
			}

			if !slices.Equal(got, testCase.wantEdges) {
				t.Fatalf("Edges() = %v, want %v", got, testCase.wantEdges)
			}
		})
	}
}

func TestPackEdgesRefuse(t *testing.T) {
	t.Parallel()

	decls := []entities.Decl{
		declOf("a.go", "A"),
		declOf("b.go", "B"),
		declOf("a_test.go", "TestA"),
		declOf("main.go", "main"),
		stmtOf("main.go", "main#0"),
	}
	testCases := []struct {
		name    string
		config  string
		wantErr error
	}{
		{
			name:    "a decl that matches nothing is a dead pattern",
			config:  `{"type":"pin","decl":"a.go:func:Nope","after":"b.go:func:B"}`,
			wantErr: ErrDeadPattern,
		},
		{
			name:    "an after that matches nothing is a dead pattern",
			config:  `{"type":"pin","decl":"a.go:func:A","after":"b.go:type:*"}`,
			wantErr: ErrDeadPattern,
		},
		{
			name:    "a declaration pinned after itself is refused",
			config:  `{"type":"pin","decl":"a.go:func:A","after":"a.go:func:A"}`,
			wantErr: ErrSelfPin,
		},
		{
			name:    "a cut function pinned after itself is refused",
			config:  `{"type":"pin","decl":"main.go:func:main","after":"main.go:func:main"}`,
			wantErr: ErrSelfPin,
		},
		{
			name:    "a production decl after a test",
			config:  `{"type":"pin","decl":"a.go:func:A","after":"a_test.go:func:TestA"}`,
			wantErr: ErrProductionAfterTest,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := packOf(t, []string{testCase.config}).
				Edges(context.Background(), entities.Snapshot{}, decls)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Edges() err = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestPackAnchors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		pack  Pack
		decls []entities.Decl
	}{
		{name: "no pins and no decls", pack: NewPack()},
		{
			name:  "a pin still anchors nothing",
			pack:  NewPack(&Rule{}),
			decls: []entities.Decl{declOf("a.go", "A")},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := testCase.pack.Anchors(
				context.Background(),
				entities.Snapshot{},
				testCase.decls,
			)
			if err != nil || len(got) != 0 {
				t.Fatalf("Anchors() = %v, %v, want nothing", got, err)
			}
		})
	}
}

func TestOrderingNamesAPinCycleWithItsReasons(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		pin         string
		back        entities.EdgeReason
		wantReasons []entities.EdgeReason
	}{
		{
			name:        "a pin against a reference",
			pin:         `{"type":"pin","decl":"b.go:func:B","after":"a.go:func:A"}`,
			back:        entities.ReasonReference,
			wantReasons: []entities.EdgeReason{entities.ReasonPin, entities.ReasonReference},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			reader := declOf("a.go", "A")
			read := declOf("b.go", "B")
			decls := []entities.Decl{reader, read}

			edges, err := packOf(t, []string{testCase.pin}).
				Edges(context.Background(), entities.Snapshot{}, decls)
			if err != nil {
				t.Fatalf("Edges() = %v", err)
			}

			edges = append(edges, entities.Edge{
				Dependent: reader.Key, Provider: read.Key, Reason: testCase.back,
			})

			result, err := ordering.New(ordering.Config{Slack: 8, RoundLimit: 2000}).
				Order(entities.NewGraph(decls, edges, nil), entities.Snapshot{})
			if !errors.Is(err, apperr.ErrIncomplete) {
				t.Fatalf("Order() err = %v, want wrapping %v", err, apperr.ErrIncomplete)
			}

			if len(result.Cycles) != 1 ||
				!slices.Equal(result.Cycles[0].Reasons, testCase.wantReasons) {
				t.Fatalf(
					"Cycles = %+v, want one with reasons %v",
					result.Cycles,
					testCase.wantReasons,
				)
			}
		})
	}
}
