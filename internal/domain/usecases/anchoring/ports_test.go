package anchoring

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"reflect"
	"slices"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestEditsOf(t *testing.T) {
	t.Parallel()

	span := entities.Span{Start: 2, End: 9}
	testCases := []struct {
		name   string
		active map[entities.DeclKey]entry
		want   []Edit
	}{
		{name: "nothing held", want: []Edit{}},
		{
			name: "declarations sharing one span give one edit",
			active: map[entities.DeclKey]entry{
				fkey("A"): {key: fkey("A"), span: span, preimage: "const ( A; B )"},
				fkey("B"): {key: fkey("B"), span: span, preimage: "const ( A; B )"},
			},
			want: []Edit{{File: "a.go", Span: span, Text: "const ( A; B )"}},
		},
		{
			name: "declarations sharing one span with differing text keep both edits",
			active: map[entities.DeclKey]entry{
				fkey("A"): {key: fkey("A"), span: span, preimage: "var a = 1"},
				fkey("B"): {key: fkey("B"), span: span, preimage: "var b = 2"},
			},
			want: []Edit{
				{File: "a.go", Span: span, Text: "var a = 1"},
				{File: "a.go", Span: span, Text: "var b = 2"},
			},
		},
		{
			name: "distinct spans stay in file order",
			active: map[entities.DeclKey]entry{
				fkey("B"): {key: fkey("B"), span: entities.Span{Start: 20, End: 25}, preimage: "b"},
				fkey("A"): {key: fkey("A"), span: entities.Span{Start: 1, End: 5}, preimage: "a"},
			},
			want: []Edit{
				{File: "a.go", Span: entities.Span{Start: 1, End: 5}, Text: "a"},
				{File: "a.go", Span: entities.Span{Start: 20, End: 25}, Text: "b"},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := editsOf(testCase.active, nil)
			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("editsOf() = %+v, want %+v", got, testCase.want)
			}
		})
	}
}

func TestEditsOfCarryTheImports(t *testing.T) {
	t.Parallel()

	active := map[entities.DeclKey]entry{
		fkey("A"): {
			key: fkey("A"), span: entities.Span{End: 6}, preimage: "A one",
			imports: []string{"bytes"},
		},
	}
	want := []Edit{{
		File: "a.go", Span: entities.Span{End: 6}, Text: "A one", Imports: []string{"bytes"},
		Keep: []entities.DeclKey{fkey("A")},
	}}

	got := editsOf(active, map[string][]entities.DeclKey{"a.go": {fkey("A")}})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("editsOf() = %+v, want %+v", got, want)
	}
}

func TestStopSummary(t *testing.T) {
	t.Parallel()

	stopping := Anchor{Rev: "fix", Position: 5, Policy: PolicyStop}

	testCases := []struct {
		name        string
		verdict     Verdict
		wantOutcome string
		wantStops   int
	}{
		{
			name: "a conflict is summarised as stopped", verdict: Verdict{Detail: "undefined: Z"},
			wantOutcome: "stopped", wantStops: 1,
		},
		{
			name:        "an unfit pre-image under stop is summarised as folded",
			verdict:     Verdict{IsUnfit: true, Detail: "cannot be rendered"},
			wantOutcome: "folded",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var (
				record probeLog
				buf    bytes.Buffer
			)

			ctrl := gomock.NewController(t)
			probe := scripted(ctrl, &record, []Verdict{testCase.verdict}, nil)
			planner := fixture(ctrl, probe, "", stopping)
			planner.cfg.Logger = slog.New(slog.NewJSONHandler(&buf, nil))

			_, err := planOf(planner, finalFiles())
			if (testCase.wantStops > 0) != errors.Is(err, ErrStopped) {
				t.Fatalf("Plan() = %v, want %d stops", err, testCase.wantStops)
			}

			var outcomes []any

			for _, fields := range logged(t, &buf) {
				if fields["msg"] == "anchors: summary" {
					outcomes = append(outcomes, fields["outcome"])
				}
			}

			if !slices.Equal(outcomes, []any{testCase.wantOutcome}) {
				t.Fatalf("summary outcomes = %v, want [%s]", outcomes, testCase.wantOutcome)
			}
		})
	}
}

func TestDecideForgetsTheStopsOfAnEarlierRound(t *testing.T) {
	t.Parallel()

	var record probeLog

	ctrl := gomock.NewController(t)
	probe := scripted(ctrl, &record, []Verdict{{Detail: "undefined: Z"}, {IsBuilt: true}}, nil)
	planner := fixture(ctrl, probe, PolicyStop, Anchor{Rev: "fix", Position: 5})
	graph := graphOf(finalFiles())

	list, err := planner.collect(context.Background(), graph, placedIn(basePlan()))
	if err != nil {
		t.Fatalf("collect() = %v", err)
	}

	judge := newJudgement()

	for round, wantStops := range []int{1, 0} {
		if _, err = planner.decide(context.Background(), graph, list, judge); err != nil {
			t.Fatalf("decide() round %d = %v", round, err)
		}

		if len(judge.stops) != wantStops {
			t.Fatalf("round %d holds %d stops, want %d", round, len(judge.stops), wantStops)
		}

		clear(judge.memo)
	}
}
