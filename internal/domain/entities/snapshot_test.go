package entities

import "testing"

func TestSnapshotHasHistory(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		snap Snapshot
		want bool
	}{
		{"bare tree", Snapshot{Files: []string{"a.go"}}, false},
		{
			"with history",
			Snapshot{History: []SourceTouch{{Position: 0, Touched: []string{"a.go"}}}},
			true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.snap.HasHistory(); got != testCase.want {
				t.Fatalf("HasHistory() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestSnapshotFirstTouch(t *testing.T) {
	t.Parallel()

	snap := Snapshot{History: []SourceTouch{
		{Position: 0, Touched: []string{"go.mod"}},
		{Position: 4, Touched: []string{"a.go", "b.go"}},
		{Position: 9, Touched: []string{"a.go"}},
	}}

	testCases := []struct {
		name string
		snap Snapshot
		path string
		want Position
	}{
		{"earliest wins, not the latest", snap, "a.go", 4},
		{"single touch", snap, "go.mod", 0},
		{"never touched", snap, "c.go", NoPosition},
		{
			"an unsorted history still yields the true minimum, not the first match",
			Snapshot{History: []SourceTouch{
				{Position: 9, Touched: []string{"a.go"}},
				{Position: 2, Touched: []string{"a.go"}},
				{Position: 5, Touched: []string{"a.go"}},
			}},
			"a.go",
			2,
		},
		{"no history at all", Snapshot{}, "a.go", NoPosition},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.snap.FirstTouch(testCase.path); got != testCase.want {
				t.Fatalf("FirstTouch(%q) = %d, want %d", testCase.path, got, testCase.want)
			}
		})
	}
}
