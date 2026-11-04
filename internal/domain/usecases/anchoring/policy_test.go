package anchoring

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type policyCase struct {
	name      string
	anchors   []Anchor
	policy    Policy
	verdicts  []Verdict
	failure   error
	wantSize  int
	wantWIP   []int
	wantCalls int
	wantErr   error
}

func runPolicy(t *testing.T, testCase policyCase) {
	t.Helper()

	var record probeLog

	probe := scripted(gomock.NewController(t), &record, testCase.verdicts, testCase.failure)
	planner := fixture(gomock.NewController(t), probe, testCase.policy, testCase.anchors...)

	got, err := planOf(planner, finalFiles())
	if !errors.Is(err, testCase.wantErr) {
		t.Fatalf("Plan() error = %v, want %v", err, testCase.wantErr)
	}

	if testCase.failure != nil && !errors.Is(err, testCase.failure) {
		t.Fatalf("Plan() error = %v, want it to wrap %v", err, testCase.failure)
	}

	if len(record.edits) != testCase.wantCalls {
		t.Fatalf("probe calls = %d, want %d", len(record.edits), testCase.wantCalls)
	}

	if testCase.wantErr != nil {
		return
	}

	if got.Len() != testCase.wantSize {
		t.Fatalf("commits = %d, want %d", got.Len(), testCase.wantSize)
	}

	var wip []int

	for idx, commit := range got.Commits {
		if commit.IsWIP {
			wip = append(wip, idx)
		}
	}

	if !slices.Equal(wip, testCase.wantWIP) {
		t.Fatalf("wip commits = %v, want %v", wip, testCase.wantWIP)
	}
}

func TestPolicyVerdicts(t *testing.T) {
	t.Parallel()

	built := []Verdict{{IsBuilt: true}}
	broken := []Verdict{{Detail: "undefined: Z"}}
	unchecked := []Verdict{{IsUnchecked: true, Detail: "fails before any edit"}}
	unfit := []Verdict{{IsUnfit: true, Detail: "pre-image cannot be rendered"}}
	fix := Anchor{Rev: "fix", Position: 5}
	wip := Anchor{Rev: "fix", Position: 5, Policy: PolicyWIP}

	testCases := []policyCase{
		{
			name: "a pre-image that compiles is kept", anchors: []Anchor{fix},
			verdicts: built, wantSize: 3, wantCalls: 1,
		},
		{
			name:     "fold is the default and drops an anchor that does not build",
			anchors:  []Anchor{fix},
			verdicts: broken, wantSize: 2, wantCalls: 1,
		},
		{
			name:     "wip keeps the anchor and marks the commits carrying the pre-image",
			anchors:  []Anchor{wip},
			verdicts: broken, wantSize: 3, wantWIP: []int{0}, wantCalls: 1,
		},
		{
			name: "the global policy applies to an anchor that names none", anchors: []Anchor{fix},
			policy: PolicyWIP, verdicts: broken, wantSize: 3, wantWIP: []int{0}, wantCalls: 1,
		},
		{
			name:    "an anchor policy overrides the global one",
			anchors: []Anchor{{Rev: "fix", Position: 5, Policy: PolicyFold}}, policy: PolicyWIP,
			verdicts: broken, wantSize: 2, wantCalls: 1,
		},
		{
			name:     "an unchecked anchor is kept as a real commit, neither folded nor wip",
			anchors:  []Anchor{fix},
			verdicts: unchecked, wantSize: 3, wantCalls: 1,
		},
		{
			name:     "an unfit pre-image folds the anchor under the default policy",
			anchors:  []Anchor{fix},
			verdicts: unfit, wantSize: 2, wantCalls: 1,
		},
		{
			name:     "an unfit pre-image folds the anchor under wip too",
			anchors:  []Anchor{wip},
			verdicts: unfit, wantSize: 2, wantCalls: 1,
		},
		{
			name:    "an unchecked anchor stays unmarked under the wip policy",
			anchors: []Anchor{wip}, verdicts: unchecked, wantSize: 3, wantCalls: 1,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			runPolicy(t, testCase)
		})
	}
}

func TestPolicyRefusals(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	built := []Verdict{{IsBuilt: true}}
	broken := []Verdict{{Detail: "undefined: Z"}}

	testCases := []policyCase{
		{
			name:    "an anchor that changes nothing is reported and dropped without a probe",
			anchors: []Anchor{{Rev: "noop", Position: 5}}, verdicts: broken, wantSize: 2,
		},
		{
			name:     "a probe that cannot run is an error, not a fold",
			anchors:  []Anchor{{Rev: "fix", Position: 5}},
			verdicts: broken, failure: boom, wantCalls: 1, wantErr: ErrProbe,
		},
		{
			name:    "a policy the planner does not know is refused",
			anchors: []Anchor{{Rev: "fix", Position: 5, Policy: "stop"}}, verdicts: built,
			wantErr: ErrAnchor,
		},
		{
			name:    "an anchor whose revision cannot be read is refused",
			anchors: []Anchor{{Rev: "gone", Position: 5}}, verdicts: built, wantErr: ErrAnchor,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			runPolicy(t, testCase)
		})
	}
}

func TestPolicyTrialsHoldOnlyAnchorsJudgedBuilt(t *testing.T) {
	t.Parallel()

	built := Verdict{IsBuilt: true}
	testCases := []struct {
		name      string
		newer     Verdict
		wantEdits int
	}{
		{name: "a newer anchor that builds is in the older trial", newer: built, wantEdits: 2},
		{
			name:  "a newer wip anchor is left out of the older trial",
			newer: Verdict{Detail: "no"}, wantEdits: 1,
		},
		{
			name:  "a newer unchecked anchor is left out of the older trial",
			newer: Verdict{IsUnchecked: true}, wantEdits: 1,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var record probeLog

			ctrl := gomock.NewController(t)
			probe := scripted(ctrl, &record, []Verdict{testCase.newer, built}, nil)
			newer := Anchor{Rev: "fixb", Position: 7, Policy: PolicyWIP}
			older := Anchor{Rev: "fix", Position: 5}

			got, err := planOf(fixture(ctrl, probe, "", older, newer), finalFiles())
			if err != nil {
				t.Fatalf("Plan() = %v", err)
			}

			if len(record.edits) != 2 || len(record.edits[1]) != testCase.wantEdits {
				t.Fatalf(
					"probe edits = %+v, want the older trial to hold %d",
					record.edits, testCase.wantEdits,
				)
			}

			if got.Len() != 4 {
				t.Fatalf("commits = %d, want both anchors kept", got.Len())
			}
		})
	}
}

func TestWIPSubjectsAreMarkedButTheAnchorKeepsItsOwn(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		anchor   Anchor
		verdicts []Verdict
		want     []string
	}{
		{
			name:     "a wip anchor marks the feature commits but keeps its own subject",
			anchor:   Anchor{Rev: "fix", Position: 5, Policy: PolicyWIP},
			verdicts: []Verdict{{Detail: "broken"}},
			want: []string{
				entities.MarkWIP("feat(a): add A"), "fix(golang): repair A", "feat(b): add B",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var record probeLog

			ctrl := gomock.NewController(t)
			probe := scripted(ctrl, &record, testCase.verdicts, nil)
			planner := fixture(ctrl, probe, "", testCase.anchor)

			got, err := planOf(planner, finalFiles())
			if err != nil {
				t.Fatalf("Plan() = %v", err)
			}

			for idx, commit := range got.Commits {
				if commit.Subject != testCase.want[idx] {
					t.Fatalf("subject %d = %q, want %q", idx, commit.Subject, testCase.want[idx])
				}
			}
		})
	}
}

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
