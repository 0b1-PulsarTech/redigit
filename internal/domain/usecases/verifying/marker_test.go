package verifying

import "testing"

func TestSweepWipSubjects(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		subject string
		wantWip bool
	}{
		{"a marker after the type and scope", "feat(a): [wip] s", true},
		{"a marker after the draft prefix", "DRAFT: feat(a): [wip] s", true},
		{"a marker leading a plain subject", "[wip] s", true},
		{"a subject that only mentions wip", "feat(a): fix wip parsing", false},
		{"no marker", "feat(a): s", false},
		{"a marker explained mid-description", "docs(a): explain the [wip] marker", false},
		{"a marker after a second colon", "feat(a): add x: [wip] y", false},
		{"a marker after the draft prefix and no type", "DRAFT: [wip] s", true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fixture := newRig(1, testModules())
			fixture.builder.failing = map[string]string{"c01/beta": "undefined: x"}

			commits := fixture.sweep.Commits.(fakeCommits)
			commits.list[0].Subject = testCase.subject

			_, summary, err := collect(fixture.sweep, Options{})
			if err != nil {
				t.Fatalf("run: %v", err)
			}

			if gotWip := summary.WipFailed == 1; gotWip != testCase.wantWip ||
				summary.Failed+summary.WipFailed != 1 {
				t.Fatalf("summary = %+v, want wip %v", summary, testCase.wantWip)
			}
		})
	}
}
