package anchoring

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestValidate(t *testing.T) {
	t.Parallel()

	fix := Spec{Rev: "3d2d883", Kind: "fix"}

	testCases := []struct {
		name     string
		specs    []Spec
		policy   Policy
		wantErr  string
		wantText string
	}{
		{name: "no anchor is fine"},
		{name: "a fix and a perf", specs: []Spec{fix, {Rev: "0aab4c6", Kind: "perf"}}},
		{name: "a global wip policy", specs: []Spec{fix}, policy: PolicyWIP},
		{
			name:  "an anchor policy",
			specs: []Spec{{Rev: "3d2d883", Kind: "fix", Policy: PolicyFold}},
		},
		{
			name: "stop is for the next slice", policy: "stop",
			wantErr: "arrives in 24b", wantText: "policy stop arrives in 24b",
		},
		{
			name:    "stop on an anchor is for the next slice",
			specs:   []Spec{{Rev: "3d2d883", Kind: "fix", Policy: "stop"}},
			wantErr: "arrives in 24b", wantText: "policy stop arrives in 24b",
		},
		{name: "an unknown policy", policy: "maybe", wantErr: "unknown policy"},
		{name: "a missing rev", specs: []Spec{{Kind: "fix"}}, wantErr: "rev"},
		{name: "an unknown kind", specs: []Spec{{Rev: "3d2d883", Kind: "docs"}}, wantErr: "kind"},
		{name: "the same rev twice", specs: []Spec{fix, fix}, wantErr: "twice"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := Validate(testCase.specs, testCase.policy)
			if (err == nil) != (testCase.wantErr == "") {
				t.Fatalf("Validate() = %v, want error %q", err, testCase.wantErr)
			}

			if err == nil {
				return
			}

			if testCase.wantText != "" && err.Error() != testCase.wantText {
				t.Fatalf("Validate() = %q, want exactly %q", err, testCase.wantText)
			}

			if !errors.Is(err, ErrAnchor) || !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf("Validate() = %v, want an ErrAnchor holding %q", err, testCase.wantErr)
			}
		})
	}
}

func TestLocate(t *testing.T) {
	t.Parallel()

	history := []entities.SourceTouch{
		{Position: 0, Rev: "aaaa111100000000000000000000000000000000"},
		{Position: 1, Rev: "3d2d883000000000000000000000000000000000"},
		{Position: 2, Rev: "3d2d884000000000000000000000000000000000"},
		{Position: 3, Rev: "0aab4c6000000000000000000000000000000000"},
	}

	testCases := []struct {
		name    string
		specs   []Spec
		want    []Anchor
		wantErr bool
	}{
		{name: "no spec", want: nil},
		{
			name:  "an abbreviated rev becomes the full one at its position",
			specs: []Spec{{Rev: "0aab4c6", Policy: PolicyWIP}},
			want: []Anchor{
				{Rev: "0aab4c6000000000000000000000000000000000", Position: 3, Policy: PolicyWIP},
			},
		},
		{
			name:  "a full rev",
			specs: []Spec{{Rev: history[0].Rev}},
			want:  []Anchor{{Rev: history[0].Rev}},
		},
		{name: "an unknown rev", specs: []Spec{{Rev: "beef000"}}, wantErr: true},
		{name: "an ambiguous prefix", specs: []Spec{{Rev: "3d2d88"}}, wantErr: true},
		{
			name:    "two names for one commit",
			specs:   []Spec{{Rev: "0aab4c6"}, {Rev: "0aab4"}},
			wantErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := Locate(testCase.specs, history)
			if (err != nil) != testCase.wantErr || (err != nil && !errors.Is(err, ErrAnchor)) {
				t.Fatalf("Locate() error = %v, want an ErrAnchor = %v", err, testCase.wantErr)
			}

			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("Locate() = %+v, want %+v", got, testCase.want)
			}
		})
	}
}
