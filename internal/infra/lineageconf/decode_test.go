package lineageconf

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
)

func TestDecode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		raw         string
		wantFollow  bool
		wantPolicy  anchoring.Policy
		wantAnchors []anchoring.Spec
	}{
		{name: "a missing section follows renames", wantFollow: true},
		{name: "an empty object follows renames", raw: `{}`, wantFollow: true},
		{name: "null is no section", raw: `null`, wantFollow: true},
		{name: "follow true", raw: `{"follow": true}`, wantFollow: true},
		{name: "follow false", raw: `{"follow": false}`},
		{
			name: "anchors decode with their own policy and the global one",
			raw: `{"policy": "wip", "anchors": [
				{"rev": "3d2d883", "kind": "fix"}, {"rev": "0aab4c6", "kind": "perf", "policy": "fold"}]}`,
			wantFollow: true, wantPolicy: anchoring.PolicyWIP,
			wantAnchors: []anchoring.Spec{
				{Rev: "3d2d883", Kind: "fix"},
				{Rev: "0aab4c6", Kind: "perf", Policy: anchoring.PolicyFold},
			},
		},
		{
			name: "a global policy alone is kept", raw: `{"policy": "fold"}`,
			wantFollow: true, wantPolicy: anchoring.PolicyFold,
		},
		{
			name: "a global stop policy is kept", raw: `{"policy": "stop"}`,
			wantFollow: true, wantPolicy: anchoring.PolicyStop,
		},
		{
			name:       "stop on one anchor is kept",
			raw:        `{"anchors": [{"rev": "3d2d883", "kind": "fix", "policy": "stop"}]}`,
			wantFollow: true,
			wantAnchors: []anchoring.Spec{
				{Rev: "3d2d883", Kind: "fix", Policy: anchoring.PolicyStop},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := Decode(json.RawMessage(testCase.raw))
			if err != nil {
				t.Fatalf("Decode() = %v", err)
			}

			if got.Follow != testCase.wantFollow || got.Policy != testCase.wantPolicy ||
				!slices.Equal(got.Anchors, testCase.wantAnchors) {
				t.Fatalf(
					"section = %+v, want follow %v policy %q anchors %v",
					got, testCase.wantFollow, testCase.wantPolicy, testCase.wantAnchors,
				)
			}
		})
	}
}

func TestDecodeRefusals(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		raw  string
	}{
		{name: "an unknown policy", raw: `{"policy": "later"}`},
		{name: "an unknown kind", raw: `{"anchors": [{"rev": "3d2d883", "kind": "feat"}]}`},
		{name: "a missing rev", raw: `{"anchors": [{"kind": "fix"}]}`},
		{
			name: "a repeated rev",
			raw:  `{"anchors": [{"rev": "3d2d883", "kind": "fix"}, {"rev": "3d2d883", "kind": "perf"}]}`,
		},
		{name: "anchors that are not a list", raw: `{"anchors": {"policy": "fold"}}`},
		{
			name: "an unknown anchor key",
			raw:  `{"anchors": [{"rev": "3d2d883", "kind": "fix", "slot": 3}]}`,
		},
		{name: "an unknown key", raw: `{"rename": true}`},
		{name: "a wrong type", raw: `{"follow": "yes"}`},
		{name: "a non-object", raw: `[true]`},
		{name: "trailing data", raw: `{} {}`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := Decode(json.RawMessage(testCase.raw))
			if err == nil {
				t.Fatal("Decode() = nil, want a refusal")
			}
		})
	}
}

func TestIsStopping(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		section Section
		want    bool
	}{
		{name: "an empty section", section: Section{}},
		{name: "a global stop", section: Section{Policy: anchoring.PolicyStop}, want: true},
		{
			name: "an anchor stop under a global fold",
			section: Section{
				Policy:  anchoring.PolicyFold,
				Anchors: []anchoring.Spec{{Rev: "a"}, {Rev: "b", Policy: anchoring.PolicyStop}},
			},
			want: true,
		},
		{
			name: "no stop anywhere",
			section: Section{
				Policy:  anchoring.PolicyWIP,
				Anchors: []anchoring.Spec{{Rev: "a", Policy: anchoring.PolicyFold}},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.section.IsStopping(); got != testCase.want {
				t.Fatalf("IsStopping() = %v, want %v", got, testCase.want)
			}
		})
	}
}
