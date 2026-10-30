package bootstrap

import (
	"errors"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/infra/rules"
)

func TestRuleHook(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		hook stageHook
	}{
		{"leaves the stages unchanged", ruleHook},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			hookLeavesTheStagesUnchanged(t, testCase.hook)
		})
	}
}

func TestRuleConstructors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		specs    []rules.Spec
		wantErr  error
		wantText string
	}{
		{name: "no rules build to nothing"},
		{
			name:     "a rule type is refused while none is registered",
			specs:    []rules.Spec{{Type: "go-di"}},
			wantErr:  rules.ErrUnknownType,
			wantText: "no rule types are registered",
		},
	}

	if got := len(ruleConstructors()); got != 0 {
		t.Fatalf("len(ruleConstructors()) = %d, want 0", got)
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			built, err := compiledRegistry().Build(testCase.specs)

			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Build() err = %v, want %v", err, testCase.wantErr)
			}

			if testCase.wantText != "" && !strings.Contains(err.Error(), testCase.wantText) {
				t.Fatalf("Build() err = %q, want it to say %q", err, testCase.wantText)
			}

			if err == nil && len(built) != 0 {
				t.Fatalf("Build() = %v, want no rules", built)
			}
		})
	}
}
