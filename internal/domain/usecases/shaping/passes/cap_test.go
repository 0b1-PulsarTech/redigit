package passes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/shaping"
)

func capOf(t testing.TB) Cap {
	t.Helper()

	pass, err := NewCap(json.RawMessage(`{"type":"finish-cap","maxAdded":100}`))
	if err != nil {
		t.Fatalf("NewCap() = %v", err)
	}

	return pass
}

func filesByRow(draft *shaping.Draft) [][]string {
	plan := draft.Plan()
	rows := make([][]string, 0, len(plan.Commits))

	for _, commit := range plan.Commits {
		files := make([]string, 0, len(commit.Decls))
		for _, key := range commit.Decls {
			files = append(files, key.File)
		}

		rows = append(rows, files)
	}

	return rows
}

func applyCap(t *testing.T, rows [][]declSpec, links []linkSpec, want [][]string) {
	t.Helper()

	pass := capOf(t)
	draft := synth(rows, nil, links...)
	before := len(draft.Violations())

	if _, err := pass.Apply(t.Context(), draft); err != nil {
		t.Fatalf("Apply() = %v", err)
	}

	if got := filesByRow(draft); !reflect.DeepEqual(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}

	if after := len(draft.Violations()); after > before {
		t.Fatalf("violations = %d, had %d", after, before)
	}

	settled := draft.Plan()

	again, err := pass.Apply(t.Context(), draft)
	if err != nil || len(again) != 0 {
		t.Fatalf("second Apply() = %d moves, %v, want 0, nil", len(again), err)
	}

	if !reflect.DeepEqual(settled, draft.Plan()) {
		t.Fatalf("second Apply() changed the plan: %v", draft.Plan())
	}
}

func TestCapLeavesARowAsItIs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		rows      [][]declSpec
		links     []linkSpec
		wantFiles [][]string
	}{
		{
			name: "a row within the cap stays whole",
			rows: [][]declSpec{{
				{file: "a.go", name: "A", lines: 60},
				{file: "a_test.go", name: "TestA", lines: 40, test: true},
			}},
			wantFiles: [][]string{{"a.go", "a_test.go"}},
		},
		{
			name: "a row of production files only stays as it is",
			rows: [][]declSpec{{
				{file: "a.go", name: "A", lines: 50},
				{file: "b.go", name: "B", lines: 50},
				{file: "c.go", name: "C", lines: 50},
			}},
			wantFiles: [][]string{{"a.go", "b.go", "c.go"}},
		},
		{
			name: "a single file past the cap has nothing to hand over",
			rows: [][]declSpec{{
				{file: "a_test.go", name: "TestA", lines: 90, test: true},
				{file: "a_test.go", name: "TestB", lines: 90, test: true},
			}},
			wantFiles: [][]string{{"a_test.go", "a_test.go"}},
		},
		{
			name: "a test file whose consumers are the whole row stays",
			rows: [][]declSpec{{
				{file: "a_test.go", name: "TestA", lines: 90, test: true},
				{file: "b_test.go", name: "TestB", lines: 90, test: true},
			}},
			links: []linkSpec{
				{edgeSpec{"TestA", "TestB"}, entities.ReasonReference},
				{edgeSpec{"TestB", "TestA"}, entities.ReasonReference},
			},
			wantFiles: [][]string{{"a_test.go", "b_test.go"}},
		},
		{
			name: "test files an earlier row already consumes stay",
			rows: [][]declSpec{
				{{file: "early.go", name: "Early", lines: 5}},
				{
					{file: "a_test.go", name: "A", lines: 90, test: true},
					{file: "b_test.go", name: "B", lines: 30, test: true},
				},
			},
			links: []linkSpec{
				{edgeSpec{"Early", "A"}, entities.ReasonReference},
				{edgeSpec{"Early", "B"}, entities.ReasonReference},
			},
			wantFiles: [][]string{{"early.go"}, {"a_test.go", "b_test.go"}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			applyCap(t, testCase.rows, testCase.links, testCase.wantFiles)
		})
	}
}

func TestCapHandsOffTestFiles(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		rows      [][]declSpec
		links     []linkSpec
		wantFiles [][]string
	}{
		{
			name: "a row past the cap hands its test file to the row right after",
			rows: [][]declSpec{
				{
					{file: "a.go", name: "A", lines: 80},
					{file: "a_test.go", name: "TestA", lines: 40, test: true},
				},
				{{file: "z.go", name: "Z", lines: 10}},
			},
			links:     []linkSpec{{edgeSpec{"TestA", "A"}, entities.ReasonReference}},
			wantFiles: [][]string{{"a.go"}, {"a_test.go"}, {"z.go"}},
		},
		{
			name: "the largest test file leaves first and one is enough",
			rows: [][]declSpec{{
				{file: "a.go", name: "A", lines: 70},
				{file: "small_test.go", name: "TestSmall", lines: 10, test: true},
				{file: "big_test.go", name: "TestBig", lines: 30, test: true},
			}},
			wantFiles: [][]string{{"a.go", "small_test.go"}, {"big_test.go"}},
		},
		{
			name: "production stays past the cap once the test files have left",
			rows: [][]declSpec{{
				{file: "a.go", name: "A", lines: 70},
				{file: "b.go", name: "B", lines: 60},
				{file: "a_test.go", name: "TestA", lines: 5, test: true},
			}},
			wantFiles: [][]string{{"a.go", "b.go"}, {"a_test.go"}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			applyCap(t, testCase.rows, testCase.links, testCase.wantFiles)
		})
	}
}

func TestCapKeepsWhatTheRowsAroundItNeed(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		rows      [][]declSpec
		links     []linkSpec
		wantFiles [][]string
	}{
		{
			name: "a test file production reads never leaves",
			rows: [][]declSpec{{
				{file: "p.go", name: "Prod", lines: 60},
				{file: "help_test.go", name: "Help", lines: 60, test: true},
				{file: "other_test.go", name: "TestOther", lines: 5, test: true},
			}},
			links:     []linkSpec{{edgeSpec{"Prod", "Help"}, entities.ReasonReference}},
			wantFiles: [][]string{{"p.go", "help_test.go"}, {"other_test.go"}},
		},
		{
			name: "an atom among test files travels whole",
			rows: [][]declSpec{{
				{file: "p.go", name: "P", lines: 40},
				{file: "b_test.go", name: "B", lines: 60, test: true, atom: "tool"},
				{file: "c_test.go", name: "C", lines: 10, test: true, atom: "tool"},
			}},
			wantFiles: [][]string{{"p.go"}, {"b_test.go", "c_test.go"}},
		},
		{
			name: "a test file an earlier row consumes stays and the next one leaves",
			rows: [][]declSpec{
				{{file: "early.go", name: "Early", lines: 5}},
				{
					{file: "a_test.go", name: "A", lines: 90, test: true},
					{file: "b_test.go", name: "B", lines: 30, test: true},
				},
			},
			links:     []linkSpec{{edgeSpec{"Early", "A"}, entities.ReasonReference}},
			wantFiles: [][]string{{"early.go"}, {"a_test.go"}, {"b_test.go"}},
		},
		{
			name: "a handed off row past the cap splits again",
			rows: [][]declSpec{{
				{file: "a_test.go", name: "A", lines: 90, test: true},
				{file: "b_test.go", name: "B", lines: 80, test: true},
				{file: "c_test.go", name: "C", lines: 70, test: true},
			}},
			wantFiles: [][]string{{"c_test.go"}, {"b_test.go"}, {"a_test.go"}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			applyCap(t, testCase.rows, testCase.links, testCase.wantFiles)
		})
	}
}

func TestCapChargesAFileFrameToTheRowThatBearsIt(t *testing.T) {
	t.Parallel()

	const (
		frame = 30
		lines = 35
	)

	decls := []entities.Decl{
		{
			Key:   entities.DeclKey{File: "a_test.go", Kind: entities.KindFunc, Name: "A"},
			Lines: lines,
			Frame: frame,
		},
		{
			Key:   entities.DeclKey{File: "a_test.go", Kind: entities.KindFunc, Name: "B"},
			Lines: lines,
			Frame: frame,
		},
		{
			Key:   entities.DeclKey{File: "b_test.go", Kind: entities.KindFunc, Name: "C"},
			Lines: lines,
			Frame: frame,
		},
		{Key: entities.DeclKey{File: "c_test.go", Kind: entities.KindFunc, Name: "D"}},
	}

	testCases := []struct {
		name     string
		rows     [][]int
		wantRows int
	}{
		{name: "both frames push the row past the cap", rows: [][]int{{0, 2}}, wantRows: 2},
		{
			name:     "a file an earlier row bore charges no frame again",
			rows:     [][]int{{0}, {1, 2}},
			wantRows: 2,
		},
		{
			name:     "a file charges its frame once however many decls it holds",
			rows:     [][]int{{0, 1, 3}},
			wantRows: 1,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			plan := entities.Plan{}

			for position, row := range testCase.rows {
				commit := entities.PlannedCommit{
					Position: entities.Position(position),
					Subject:  "row",
				}
				for _, index := range row {
					commit.Decls = append(commit.Decls, decls[index].Key)
				}

				plan.Commits = append(plan.Commits, commit)
			}

			draft := shaping.NewDraft(entities.NewGraph(decls, nil, nil), plan)

			if _, err := capOf(t).Apply(t.Context(), draft); err != nil {
				t.Fatalf("Apply() = %v", err)
			}

			if got := len(draft.Plan().Commits); got != testCase.wantRows {
				t.Fatalf("rows = %d, want %d: %v", got, testCase.wantRows, draft.Plan())
			}
		})
	}
}

func TestCapStopsOnACancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	draft := synth([][]declSpec{{
		{file: "a_test.go", name: "A", lines: 90, test: true},
		{file: "b_test.go", name: "B", lines: 90, test: true},
	}}, nil)
	if _, err := capOf(t).Apply(ctx, draft); !errors.Is(err, context.Canceled) {
		t.Fatalf("Apply() = %v, want context.Canceled", err)
	}
}

func TestNewCap(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		raw     string
		want    int
		wantErr error
	}{
		{name: "the default cap", raw: `{"type":"finish-cap"}`, want: 850},
		{name: "a configured cap", raw: `{"type":"finish-cap","maxAdded":100}`, want: 100},
		{name: "unknown key", raw: `{"type":"finish-cap","nope":1}`, wantErr: shaping.ErrMalformed},
		{name: "zero", raw: `{"type":"finish-cap","maxAdded":0}`, wantErr: shaping.ErrMalformed},
		{name: "below", raw: `{"type":"finish-cap","maxAdded":-1}`, wantErr: shaping.ErrMalformed},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			pass, err := NewCap(json.RawMessage(testCase.raw))
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("NewCap() error = %v, want %v", err, testCase.wantErr)
			}

			if err != nil && !strings.Contains(err.Error(), CapName) {
				t.Fatalf("NewCap() error = %v, want the pass named", err)
			}

			if pass.cfg.MaxAdded != testCase.want {
				t.Fatalf("MaxAdded = %d, want %d", pass.cfg.MaxAdded, testCase.want)
			}
		})
	}
}

func TestCapReportsTheRowsItCannotShrink(t *testing.T) {
	testCases := []struct {
		name string
		rows [][]declSpec
		want []string
	}{
		{
			name: "a row within the cap says nothing",
			rows: [][]declSpec{{{file: "a.go", name: "A", lines: 60}}},
		},
		{
			name: "a row with no test file that can leave is named once",
			rows: [][]declSpec{
				{{file: "z.go", name: "Z", lines: 10}},
				{{file: "a.go", name: "A", lines: 70}, {file: "b.go", name: "B", lines: 60}},
			},
			want: []string{"position=1 lines=130 cap=100"},
		},
		{
			name: "a row still past the cap after its tests left is named",
			rows: [][]declSpec{{
				{file: "a.go", name: "A", lines: 120},
				{file: "a_test.go", name: "TestA", lines: 30, test: true},
			}},
			want: []string{"position=0 lines=120 cap=100"},
		},
		{
			name: "every row left over is named by its own position",
			rows: [][]declSpec{
				{{file: "a.go", name: "A", lines: 120}},
				{{file: "b.go", name: "B", lines: 130}},
			},
			want: []string{"position=0 lines=120 cap=100", "position=1 lines=130 cap=100"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var logged bytes.Buffer

			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
			t.Cleanup(func() { slog.SetDefault(previous) })

			draft := synth(testCase.rows, nil)
			if _, err := capOf(t).Apply(t.Context(), draft); err != nil {
				t.Fatalf("Apply() = %v", err)
			}

			lines := strings.Split(strings.TrimSpace(logged.String()), "\n")
			if logged.Len() == 0 {
				lines = nil
			}

			if len(lines) != len(testCase.want) {
				t.Fatalf("logged %d lines, want %d: %s", len(lines), len(testCase.want), &logged)
			}

			for index, want := range testCase.want {
				line := lines[index]
				if !strings.Contains(line, want) || !strings.Contains(line, "WARN") {
					t.Fatalf("line %d = %q, want a warning with %q", index, line, want)
				}
			}
		})
	}
}
