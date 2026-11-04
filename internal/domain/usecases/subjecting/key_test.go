package subjecting

import (
	"slices"
	"testing"
)

type resolveCase struct {
	name         string
	subjects     map[string]string
	wantFound    []bool
	wantProblems []Problem
}

func runResolve(t *testing.T, testCases []resolveCase) {
	t.Helper()

	slots := []Slot{
		{Position: 0, ID: "aa"},
		{Position: 5, ID: "bb"},
		{Position: 6, ID: "bb"},
		{Position: 7},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			hands, problems := Resolve(slots, testCase.subjects)

			found := make([]bool, 0, len(hands))
			for _, hand := range hands {
				found = append(found, hand.Found)
			}

			if !slices.Equal(found, testCase.wantFound) {
				t.Fatalf("Resolve() found = %v, want %v", found, testCase.wantFound)
			}

			if !slices.Equal(problems, testCase.wantProblems) {
				t.Fatalf("Resolve() problems = %v, want %v", problems, testCase.wantProblems)
			}
		})
	}
}

func TestResolveCanonicalKeys(t *testing.T) {
	t.Parallel()

	testCases := []resolveCase{
		{
			name:         "a signed position is stale",
			subjects:     map[string]string{"+5": "x"},
			wantFound:    []bool{false, false, false, false},
			wantProblems: []Problem{{Kind: KindStaleKey, Key: "+5"}},
		},
		{
			name:         "a zero padded position is stale",
			subjects:     map[string]string{"05": "x"},
			wantFound:    []bool{false, false, false, false},
			wantProblems: []Problem{{Kind: KindStaleKey, Key: "05"}},
		},
		{
			name:         "a negative zero position is stale",
			subjects:     map[string]string{"-0": "x"},
			wantFound:    []bool{false, false, false, false},
			wantProblems: []Problem{{Kind: KindStaleKey, Key: "-0"}},
		},
		{
			name:         "a padded position is stale",
			subjects:     map[string]string{" 5": "x"},
			wantFound:    []bool{false, false, false, false},
			wantProblems: []Problem{{Kind: KindStaleKey, Key: " 5"}},
		},
		{
			name:      "a plain position still reaches its slot",
			subjects:  map[string]string{"7": "x"},
			wantFound: []bool{false, false, false, true},
		},
	}

	runResolve(t, testCases)
}

func TestResolve(t *testing.T) {
	t.Parallel()

	testCases := []resolveCase{
		{
			name:      "position and id keys reach their slots",
			subjects:  map[string]string{"5": "x", "id:aa": "y"},
			wantFound: []bool{true, true, false, false},
		},
		{
			name:         "an unknown id is a problem",
			subjects:     map[string]string{"id:zz": "x"},
			wantFound:    []bool{false, false, false, false},
			wantProblems: []Problem{{Kind: KindUnknownID, Key: "id:zz"}},
		},
		{
			name:         "an id shared by two slots is ambiguous",
			subjects:     map[string]string{"id:bb": "x"},
			wantFound:    []bool{false, false, false, false},
			wantProblems: []Problem{{Kind: KindAmbiguousID, Key: "id:bb"}},
		},
		{
			name:         "an empty id never matches",
			subjects:     map[string]string{"id:": "x"},
			wantFound:    []bool{false, false, false, false},
			wantProblems: []Problem{{Kind: KindUnknownID, Key: "id:"}},
		},
		{
			name:         "a position outside the slots is stale",
			subjects:     map[string]string{"99": "x"},
			wantFound:    []bool{false, false, false, false},
			wantProblems: []Problem{{Kind: KindStaleKey, Key: "99"}},
		},
		{
			name:         "a key that is not a number is stale",
			subjects:     map[string]string{"first": "x"},
			wantFound:    []bool{false, false, false, false},
			wantProblems: []Problem{{Kind: KindStaleKey, Key: "first"}},
		},
		{
			name:         "position and id for one slot conflict",
			subjects:     map[string]string{"0": "x", "id:aa": "y"},
			wantFound:    []bool{false, false, false, false},
			wantProblems: []Problem{{Kind: KindConflict, Key: "0,id:aa"}},
		},
	}

	runResolve(t, testCases)
}
