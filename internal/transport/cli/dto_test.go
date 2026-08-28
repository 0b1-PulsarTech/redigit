package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/auditing"
)

func TestToUnseen(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		in       []auditing.Unseen
		wantLen  int
		wantJSON string
	}{
		{
			name:     "nothing hidden serialises as an empty array, never null",
			in:       nil,
			wantLen:  0,
			wantJSON: "[]",
		},
		{
			name: "a constraint naming no tag still serialises its tags as an array",
			in: []auditing.Unseen{
				{File: "a.go", Constraint: "//go:build ignore"},
			},
			wantLen:  1,
			wantJSON: `[{"file":"a.go","constraint":"//go:build ignore","tags":[]}]`,
		},
		{
			name: "the constraint and its tags both travel",
			in: []auditing.Unseen{
				{File: "b_test.go", Constraint: "//go:build integration", Tags: []string{"integration"}},
			},
			wantLen:  1,
			wantJSON: `[{"file":"b_test.go","constraint":"//go:build integration","tags":["integration"]}]`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := toUnseen(testCase.in)

			if len(got) != testCase.wantLen {
				t.Fatalf("len = %d, want %d", len(got), testCase.wantLen)
			}

			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			if string(encoded) != testCase.wantJSON {
				t.Fatalf("json = %s, want %s", encoded, testCase.wantJSON)
			}
		})
	}
}

// An audit that hid files and says so is the whole point of the field: a
// consumer must be able to tell "nothing was hidden" from "this tool never
// looked", so the key is present either way.
func TestAuditDTOAlwaysCarriesUnseen(t *testing.T) {
	t.Parallel()

	encoded, err := json.Marshal(auditDTO{Unseen: toUnseen(nil)})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if !strings.Contains(string(encoded), `"unseen":[]`) {
		t.Fatalf("audit output dropped the unseen key: %s", encoded)
	}
}
