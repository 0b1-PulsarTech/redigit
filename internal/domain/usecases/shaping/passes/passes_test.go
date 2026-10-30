package passes

import (
	"cmp"
	"encoding/json"
	"reflect"
	"testing"
)

func applyTwice(t *testing.T, testCase tidyCase) ([]string, int) {
	t.Helper()

	tidy, err := NewTidy(json.RawMessage(cmp.Or(testCase.config, `{"type":"tidy-test-after"}`)))
	if err != nil {
		t.Fatalf("NewTidy() = %v", err)
	}

	draft := synth(testCase.rows, testCase.edges)

	first, err := tidy.Apply(t.Context(), draft)
	if err != nil {
		t.Fatalf("first Apply() = %v", err)
	}

	settled := draft.Plan()

	again, err := tidy.Apply(t.Context(), draft)
	if err != nil || len(again) != 0 {
		t.Fatalf("second Apply() = %d moves, %v, want 0, nil", len(again), err)
	}

	if !reflect.DeepEqual(settled, draft.Plan()) {
		t.Fatalf("second Apply() changed the plan: %v", draft.Plan())
	}

	if violations := draft.Violations(); len(violations) != 0 {
		t.Fatalf("Apply() left violations: %v", violations)
	}

	subjects := make([]string, 0, len(settled.Commits))
	for _, commit := range settled.Commits {
		subjects = append(subjects, commit.Subject)
	}

	return subjects, len(first)
}
