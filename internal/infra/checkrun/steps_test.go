package checkrun

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunStepFlagsOnlyATimeout(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		step        func(log string) Step
		wantFailed  bool
		wantTimeout bool
	}{
		{
			name: "a step past its timeout is flagged",
			step: func(string) Step {
				return Step{Argv: []string{"fakecheck", "hang"}, Timeout: 200 * time.Millisecond}
			},
			wantFailed: true, wantTimeout: true,
		},
		{
			name:       "a step that fails is not a timeout",
			step:       func(log string) Step { return fake("fail", log, "undefined: x") },
			wantFailed: true,
		},
		{
			name: "a step that passes is not a timeout",
			step: func(log string) Step { return fake("pass", log) },
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			log := filepath.Join(t.TempDir(), "log")

			outcome, err := runStep(t.Context(), stepRun{
				dir: t.TempDir(), env: os.Environ(), step: testCase.step(log),
			})
			if err != nil {
				t.Fatalf("runStep() = %v", err)
			}

			isRight := outcome.IsFailed == testCase.wantFailed
			if !isRight || outcome.IsTimeout != testCase.wantTimeout {
				t.Fatalf("outcome = %+v, want failed=%v timeout=%v",
					outcome, testCase.wantFailed, testCase.wantTimeout)
			}
		})
	}
}
