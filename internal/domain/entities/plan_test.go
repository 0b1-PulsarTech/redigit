package entities

import (
	"slices"
	"testing"
)

func TestPlanFiles(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		plan Plan
		want []string
	}{
		{"empty", Plan{}, nil},
		{
			"one feature commit spanning directories reports each path once",
			Plan{Commits: []PlannedCommit{{
				Shape: ShapeFeature,
				Decls: []DeclKey{
					{File: "entities/lead.go", Kind: KindType, Name: "Lead"},
					{File: "usecases/create.go", Kind: KindFunc, Name: "Create"},
					{File: "usecases/create.go", Kind: KindType, Name: "Input"},
				},
			}}},
			[]string{"entities/lead.go", "usecases/create.go"},
		},
		{
			"a path touched by two commits is still one file",
			Plan{Commits: []PlannedCommit{
				{Decls: []DeclKey{{File: "a.go", Kind: KindType, Name: "A"}}},
				{Decls: []DeclKey{{File: "a.go", Kind: KindFunc, Name: "New"}}},
			}},
			[]string{"a.go"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := testCase.plan.Files()
			slices.Sort(got)

			if !slices.Equal(got, testCase.want) {
				t.Fatalf("Files() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestPlanFilesCountsChangedDeclarations(t *testing.T) {
	t.Parallel()

	first := DeclKey{File: "a.go", Kind: KindFunc, Name: "A"}
	second := DeclKey{File: "b.go", Kind: KindFunc, Name: "B"}

	testCases := []struct {
		name string
		plan Plan
		want []string
	}{
		{
			name: "a change counts its file like a declaration does",
			plan: Plan{Commits: []PlannedCommit{
				{Decls: []DeclKey{first}},
				{Changes: []DeclKey{first, second}},
			}},
			want: []string{"a.go", "b.go"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := testCase.plan.Files()
			slices.Sort(got)

			if !slices.Equal(got, testCase.want) {
				t.Fatalf("Files() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestPlanPreimageAt(t *testing.T) {
	t.Parallel()

	key := DeclKey{File: "a.go", Kind: KindFunc, Name: "A"}
	plan := Plan{Versions: map[DeclKey][]Version{key: {
		{Text: "first", Until: 4},
		{Text: "second", Until: 9},
	}}}

	testCases := []struct {
		name      string
		key       DeclKey
		at        Position
		want      string
		wantFound bool
	}{
		{name: "before the first anchor", key: key, at: 2, want: "first", wantFound: true},
		{
			name: "the commit before the first anchor", key: key, at: 3,
			want: "first", wantFound: true,
		},
		{
			name: "at the first anchor the final text holds", key: key, at: 4,
			want: "second", wantFound: true,
		},
		{name: "between anchors", key: key, at: 8, want: "second", wantFound: true},
		{name: "at the last anchor", key: key, at: 9, want: "", wantFound: false},
		{name: "after the last anchor", key: key, at: 20, want: "", wantFound: false},
		{name: "a declaration with no versions", key: DeclKey{File: "b.go"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, isFound := plan.PreimageAt(testCase.key, testCase.at)
			if got != testCase.want || isFound != testCase.wantFound {
				t.Fatalf(
					"PreimageAt = %q, %v, want %q, %v",
					got, isFound, testCase.want, testCase.wantFound,
				)
			}
		})
	}
}

func TestPlanLen(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		plan Plan
		want int
	}{
		{"empty", Plan{}, 0},
		{
			"unplaced declarations do not count as commits",
			Plan{
				Commits:  []PlannedCommit{{Subject: "one"}},
				Unplaced: []DeclKey{{File: "orphan.go", Kind: KindFunc, Name: "Lost"}},
			},
			1,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.plan.Len(); got != testCase.want {
				t.Fatalf("Len() = %d, want %d", got, testCase.want)
			}
		})
	}
}

func TestIsDraft(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		subject string
		want    bool
	}{
		{"generated", DraftMarker + "feat(lead): Create", true},
		{"authored", ":seedling: feat(lead): Register a lead", false},
		{"empty", "", false},
		{"the marker only counts at the start", "feat: " + DraftMarker, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := IsDraft(testCase.subject); got != testCase.want {
				t.Fatalf("IsDraft(%q) = %v, want %v", testCase.subject, got, testCase.want)
			}
		})
	}
}
