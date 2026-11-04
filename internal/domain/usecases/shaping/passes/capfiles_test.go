package passes

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/shaping"
)

func TestFilesByHandOrder(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		decls []declSpec
		want  []string
	}{
		{
			name: "the largest test file comes first",
			decls: []declSpec{
				{file: "small_test.go", name: "TestSmall", lines: 10, test: true},
				{file: "big_test.go", name: "TestBig", lines: 30, test: true},
			},
			want: []string{"big_test.go", "small_test.go"},
		},
		{
			name: "a file weighs the sum of its decls and not its last one",
			decls: []declSpec{
				{file: "big_test.go", name: "TestOne", lines: 30, test: true},
				{file: "big_test.go", name: "TestTwo", lines: 30, test: true},
				{file: "mid_test.go", name: "TestMid", lines: 40, test: true},
			},
			want: []string{"big_test.go", "mid_test.go"},
		},
		{
			name: "test files of one size go by name",
			decls: []declSpec{
				{file: "b_test.go", name: "TestB", lines: 20, test: true},
				{file: "a_test.go", name: "TestA", lines: 20, test: true},
			},
			want: []string{"a_test.go", "b_test.go"},
		},
		{
			name: "a production file is never offered",
			decls: []declSpec{
				{file: "a.go", name: "A", lines: 70},
				{file: "a_test.go", name: "TestA", lines: 5, test: true},
			},
			want: []string{"a_test.go"},
		},
		{
			name: "a file with one production decl is no test file",
			decls: []declSpec{
				{file: "mixed.go", name: "Helper", lines: 10, test: true},
				{file: "mixed.go", name: "Prod", lines: 10},
			},
			want: nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			draft := synth([][]declSpec{testCase.decls}, nil)

			var got []string

			for _, file := range filesByHandOrder(draft.Graph(), draft.Slots()[0].Commit.Decls) {
				got = append(got, file.name)
			}

			if !slices.Equal(got, testCase.want) {
				t.Fatalf("files = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestWithConsumers(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		rows  [][]declSpec
		links []linkSpec
		seeds []string
		want  []string
	}{
		{
			name: "a hard consumer in the row travels along",
			rows: [][]declSpec{{
				{file: "a_test.go", name: "A", lines: 5, test: true},
				{file: "b_test.go", name: "B", lines: 5, test: true},
				{file: "c_test.go", name: "C", lines: 5, test: true},
			}},
			links: []linkSpec{{edgeSpec{"B", "A"}, entities.ReasonReference}},
			seeds: []string{"A"},
			want:  []string{"A", "B"},
		},
		{
			name: "consumers of consumers travel too and the row's order is kept",
			rows: [][]declSpec{{
				{file: "a_test.go", name: "A", lines: 5, test: true},
				{file: "b_test.go", name: "B", lines: 5, test: true},
				{file: "c_test.go", name: "C", lines: 5, test: true},
			}},
			links: []linkSpec{
				{edgeSpec{"C", "B"}, entities.ReasonReference},
				{edgeSpec{"B", "A"}, entities.ReasonReference},
			},
			seeds: []string{"C", "A"},
			want:  []string{"A", "B", "C"},
		},
		{
			name: "a consumer held by another row is not this row's to take",
			rows: [][]declSpec{
				{{file: "a_test.go", name: "A", lines: 5, test: true}},
				{{file: "b_test.go", name: "B", lines: 5, test: true}},
			},
			links: []linkSpec{{edgeSpec{"B", "A"}, entities.ReasonReference}},
			seeds: []string{"A"},
			want:  []string{"A"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			joinedBy(t, testCase.rows, testCase.links, testCase.seeds, testCase.want)
		})
	}
}

func TestWithConsumersSoftAndAtoms(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		rows  [][]declSpec
		links []linkSpec
		seeds []string
		want  []string
	}{
		{
			name: "an implements consumer travels along",
			rows: [][]declSpec{{
				{file: "a_test.go", name: "Assert", lines: 5, test: true},
				{file: "b_test.go", name: "One", lines: 5, test: true},
			}},
			links: []linkSpec{{edgeSpec{"Assert", "One"}, entities.ReasonImplements}},
			seeds: []string{"One"},
			want:  []string{"Assert", "One"},
		},
		{
			name: "a test-of consumer stays behind",
			rows: [][]declSpec{{
				{file: "a_test.go", name: "A", lines: 5, test: true},
				{file: "b_test.go", name: "B", lines: 5, test: true},
			}},
			links: []linkSpec{{edgeSpec{"B", "A"}, entities.ReasonTestOf}},
			seeds: []string{"A"},
			want:  []string{"A"},
		},
		{
			name: "an atom travels whole",
			rows: [][]declSpec{{
				{file: "a_test.go", name: "A", lines: 5, test: true, atom: "tool"},
				{file: "b_test.go", name: "B", lines: 5, test: true, atom: "tool"},
				{file: "c_test.go", name: "C", lines: 5, test: true},
			}},
			seeds: []string{"B"},
			want:  []string{"A", "B"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			joinedBy(t, testCase.rows, testCase.links, testCase.seeds, testCase.want)
		})
	}
}

func joinedBy(t *testing.T, rows [][]declSpec, links []linkSpec, seedNames, want []string) {
	t.Helper()

	draft := synth(rows, nil, links...)
	held := draft.Slots()[0].Commit.Decls

	byName := map[string]entities.DeclKey{}

	for _, slot := range draft.Slots() {
		for _, key := range slot.Commit.Decls {
			byName[key.Name] = key
		}
	}

	seeds := make([]entities.DeclKey, 0, len(seedNames))
	for _, name := range seedNames {
		seeds = append(seeds, byName[name])
	}

	joined := withConsumers(draft, held, seeds)

	got := make([]string, 0, len(joined))
	for _, key := range joined {
		got = append(got, key.Name)
	}

	if !slices.Equal(got, want) {
		t.Fatalf("joined = %v, want %v", got, want)
	}
}

func TestCapHandsOffTestFilesWithTheirConsumers(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		rows      [][]declSpec
		links     []linkSpec
		wantFiles [][]string
	}{
		{
			name: "a test file takes the hard consumers in the row along",
			rows: [][]declSpec{{
				{file: "a.go", name: "A", lines: 40},
				{file: "help_test.go", name: "Help", lines: 60, test: true},
				{file: "use_test.go", name: "TestUse", lines: 30, test: true},
			}},
			links:     []linkSpec{{edgeSpec{"TestUse", "Help"}, entities.ReasonReference}},
			wantFiles: [][]string{{"a.go"}, {"help_test.go", "use_test.go"}},
		},
		{
			name: "an implements consumer between test files leaves with its method",
			rows: [][]declSpec{{
				{file: "p.go", name: "P", lines: 50},
				{file: "a_test.go", name: "Assert", lines: 10, test: true},
				{file: "b_test.go", name: "One", lines: 60, test: true},
			}},
			links:     []linkSpec{{edgeSpec{"Assert", "One"}, entities.ReasonImplements}},
			wantFiles: [][]string{{"p.go"}, {"a_test.go", "b_test.go"}},
		},
		{
			name: "a test-of consumer is soft and stays behind",
			rows: [][]declSpec{{
				{file: "p.go", name: "P", lines: 30},
				{file: "big_test.go", name: "Big", lines: 70, test: true},
				{file: "small_test.go", name: "Small", lines: 20, test: true},
			}},
			links:     []linkSpec{{edgeSpec{"Small", "Big"}, entities.ReasonTestOf}},
			wantFiles: [][]string{{"p.go", "small_test.go"}, {"big_test.go"}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			applyCap(t, testCase.rows, testCase.links, testCase.wantFiles)
		})
	}
}

func marked(draft *shaping.Draft, mark entities.PlannedCommit) *shaping.Draft {
	plan := draft.Plan()
	plan.Commits[0].Subject = mark.Subject
	plan.Commits[0].Shape = mark.Shape
	plan.Commits[0].Feature = mark.Feature
	plan.Commits[0].IsWIP = mark.IsWIP

	return shaping.NewDraft(draft.Graph(), plan)
}

func TestTitleOfTheRowsCapChanged(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		rows         [][]declSpec
		mark         entities.PlannedCommit
		wantSubjects []string
	}{
		{
			name: "a source row names what it still holds",
			rows: [][]declSpec{{
				{file: "p.go", name: "Prod", lines: 60},
				{file: "a_test.go", name: "TestA", lines: 80, test: true},
			}},
			mark: entities.PlannedCommit{
				Subject: "DRAFT: feat(pkg): Prod, TestA",
				Shape:   entities.ShapeFeature,
			},
			wantSubjects: []string{"DRAFT: feat(pkg): Prod", "DRAFT: test(pkg): TestA"},
		},
		{
			name: "names past the third are counted",
			rows: [][]declSpec{{
				{file: "p.go", name: "P1", lines: 10},
				{file: "p.go", name: "P2", lines: 10},
				{file: "p.go", name: "P3", lines: 10},
				{file: "p.go", name: "P4", lines: 10},
				{file: "p.go", name: "P5", lines: 10},
				{file: "a_test.go", name: "TestA", lines: 80, test: true},
			}},
			mark: entities.PlannedCommit{Subject: "DRAFT: feat(pkg): P1, P2, P3 and 3 more"},
			wantSubjects: []string{
				"DRAFT: feat(pkg): P1, P2, P3 and 2 more", "DRAFT: test(pkg): TestA",
			},
		},
		{
			name: "a layer row keeps its chore kind",
			rows: [][]declSpec{{
				{file: "p.go", name: "Prod", lines: 60},
				{file: "a_test.go", name: "TestA", lines: 80, test: true},
			}},
			mark: entities.PlannedCommit{
				Subject: "DRAFT: chore(shared): Prod, TestA",
				Shape:   entities.ShapeLayer,
			},
			wantSubjects: []string{"DRAFT: chore(shared): Prod", "DRAFT: test(shared): TestA"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			subjectsAfterCap(t, testCase.rows, testCase.mark, testCase.wantSubjects)
		})
	}
}

func TestTitleOfTheKindAndTheScope(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		rows         [][]declSpec
		mark         entities.PlannedCommit
		wantSubjects []string
	}{
		{
			name: "a source row left with tests only is a test row",
			rows: [][]declSpec{{
				{file: "a_test.go", name: "TestA", lines: 80, test: true},
				{file: "b_test.go", name: "TestB", lines: 60, test: true},
			}},
			mark: entities.PlannedCommit{
				Subject: "DRAFT: feat(pkg): TestA, TestB",
				Shape:   entities.ShapeFeature,
			},
			wantSubjects: []string{"DRAFT: test(pkg): TestB", "DRAFT: test(pkg): TestA"},
		},
		{
			name: "a row at the repo root is scoped shared and not dot",
			rows: [][]declSpec{{
				{file: "p.go", name: "Prod", lines: 60},
				{file: "a_test.go", name: "TestA", lines: 80, test: true},
			}},
			mark:         entities.PlannedCommit{Subject: "row a"},
			wantSubjects: []string{"row a", "DRAFT: test(shared): TestA"},
		},
		{
			name: "a feature path is scoped by its last segment",
			rows: [][]declSpec{{
				{file: "p.go", name: "Prod", lines: 60},
				{file: "a_test.go", name: "TestA", lines: 80, test: true},
			}},
			mark:         entities.PlannedCommit{Subject: "row a", Feature: "internal/api"},
			wantSubjects: []string{"row a", "DRAFT: test(api): TestA"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			subjectsAfterCap(t, testCase.rows, testCase.mark, testCase.wantSubjects)
		})
	}
}

func subjectsAfterCap(
	t *testing.T, rows [][]declSpec, mark entities.PlannedCommit, want []string,
) {
	t.Helper()

	draft := marked(synth(rows, nil), mark)
	if _, err := capOf(t).Apply(t.Context(), draft); err != nil {
		t.Fatalf("Apply() = %v", err)
	}

	commits := draft.Plan().Commits

	got := make([]string, 0, len(commits))
	for _, commit := range commits {
		got = append(got, commit.Subject)
	}

	if !slices.Equal(got, want) {
		t.Fatalf("subjects = %q, want %q", got, want)
	}
}

func TestHandedOffRowKeepsTheFieldsOfItsSource(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		mark entities.PlannedCommit
	}{
		{name: "a layer row", mark: entities.PlannedCommit{Shape: entities.ShapeLayer}},
		{
			name: "a feature row of a feature",
			mark: entities.PlannedCommit{Shape: entities.ShapeFeature, Feature: "internal/api"},
		},
		{name: "a work in progress row", mark: entities.PlannedCommit{IsWIP: true}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			draft := marked(synth([][]declSpec{{
				{file: "p.go", name: "Prod", lines: 60},
				{file: "a_test.go", name: "TestA", lines: 80, test: true},
			}}, nil), testCase.mark)

			if _, err := capOf(t).Apply(t.Context(), draft); err != nil {
				t.Fatalf("Apply() = %v", err)
			}

			commits := draft.Plan().Commits
			if len(commits) != 2 {
				t.Fatalf("rows = %d, want 2", len(commits))
			}

			spawned := commits[1]
			if spawned.Shape != testCase.mark.Shape ||
				spawned.Feature != testCase.mark.Feature ||
				spawned.IsWIP != testCase.mark.IsWIP {
				t.Fatalf("spawned = %+v, want the fields of %+v", spawned, testCase.mark)
			}
		})
	}
}
