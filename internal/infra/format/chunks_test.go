package format

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestChunks(t *testing.T) {
	testCases := []struct {
		name  string
		size  int
		paths []string
		want  [][]int
	}{
		{
			"distinct paths share a batch",
			10,
			[]string{"a.go", "b.go", "d/c.go"},
			[][]int{{0, 1, 2}},
		},
		{"the batch size caps a batch", 2, []string{"a.go", "b.go", "c.go"}, [][]int{{0, 1}, {2}}},
		{"a repeat splits the batch", 10, []string{"a.go", "b.go", "a.go"}, [][]int{{0, 1}, {2}}},
		{"a file then a path below it splits", 10, []string{"a", "a/b.go"}, [][]int{{0}, {1}}},
		{
			"a path below a name then the file splits",
			10,
			[]string{"a/b/c.go", "a"},
			[][]int{{0}, {1}},
		},
		{
			"a deep path under a file splits",
			10,
			[]string{"x.go", "a", "a/b/c.go"},
			[][]int{{0, 1}, {2}},
		},
		{"a case alias splits the batch", 10, []string{"Foo.go", "foo.go"}, [][]int{{0}, {1}}},
		{
			"a case alias of a directory splits",
			10,
			[]string{"Dir", "dir/x.go"},
			[][]int{{0}, {1}},
		},
		{
			"siblings sharing a directory stay together",
			10,
			[]string{"a/b.go", "a/c.go"},
			[][]int{{0, 1}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			jobs := make([]job, len(testCase.paths))
			for index, name := range testCase.paths {
				jobs[index] = job{path: name, clean: path.Clean(name)}
			}

			got := (&Engine{batchSize: testCase.size}).chunks(tool{}, jobs, argBudget)
			if !slices.EqualFunc(got, testCase.want, slices.Equal) {
				t.Errorf("chunks = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestChunksRunTogether(t *testing.T) {
	testCases := []formatCase{
		{
			name:  "the same path twice goes to separate runs",
			mode:  ModeFiles,
			files: []File{{"a.go", []byte("one")}, {"a.go", []byte("two")}},
			want:  []string{"ONE", "TWO"},
			exact: 2,
		},
		{
			name:  "paths differing only by case go to separate runs",
			mode:  ModeFiles,
			files: []File{{"Foo.go", []byte("one")}, {"foo.go", []byte("two")}},
			want:  []string{"ONE", "TWO"},
			exact: 2,
		},
		{
			name:  "a dot-slash spelling collides with the plain one",
			mode:  ModeFiles,
			files: []File{{"a.go", []byte("one")}, {"./a.go", []byte("two")}},
			want:  []string{"ONE", "TWO"},
			exact: 2,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, testCase.run)
	}
}

func TestChunksAlone(t *testing.T) {
	testCases := []struct {
		name  string
		alone []bool
		want  [][]int
	}{
		{"an isolated job sits in the middle", []bool{false, true, false}, [][]int{{0}, {1}, {2}}},
		{"an isolated job leads", []bool{true, false, false}, [][]int{{0}, {1, 2}}},
		{"an isolated job trails", []bool{false, false, true}, [][]int{{0, 1}, {2}}},
		{"no isolated job changes nothing", []bool{false, false, false}, [][]int{{0, 1, 2}}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			jobs := make([]job, len(testCase.alone))

			for index, alone := range testCase.alone {
				name := string(rune('a'+index)) + ".go"
				jobs[index] = job{path: name, clean: name, alone: alone}
			}

			got := (&Engine{batchSize: 10}).chunks(tool{}, jobs, argBudget)
			if !slices.EqualFunc(got, testCase.want, slices.Equal) {
				t.Errorf("chunks = %v, want %v", got, testCase.want)
			}
		})
	}
}

func withConfigBlob(isolate ...string) func(*testing.T, *Config) {
	return func(t *testing.T, config *Config) {
		t.Helper()

		config.Languages[0].Extensions = []string{".go", ".cfg"}
		config.Languages[0].Isolate = isolate
	}
}

func copied(t *testing.T, config *Config) {
	t.Helper()

	withConfigBlob()(t, config)

	config.ConfigRoot, config.CopyFiles = t.TempDir(), []string{"fake.cfg"}

	if err := os.WriteFile(
		filepath.Join(config.ConfigRoot, "fake.cfg"),
		[]byte("x"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
}

func TestIsolation(t *testing.T) {
	configBlob := File{"fake.cfg", []byte("LOWER")}
	sibling := File{"a.go", []byte("Ab")}

	testCases := []formatCase{
		{
			name:    "without isolation a config blob steers its sibling",
			mode:    ModeFiles,
			prepare: withConfigBlob(),
			files:   []File{sibling, configBlob},
			want:    []string{"ab", "lower"},
			exact:   1,
		},
		{
			name:    "an isolate glob keeps the sibling untouched",
			mode:    ModeFiles,
			prepare: withConfigBlob("fake.*"),
			files:   []File{sibling, configBlob},
			want:    []string{"AB", "lower"},
			exact:   2,
		},
		{
			name:    "an isolate glob matches the base name in a subdirectory",
			mode:    ModeFiles,
			prepare: withConfigBlob("fake.cfg"),
			files:   []File{{"sub/a.go", []byte("Ab")}, {"sub/FAKE.cfg", []byte("LOWER")}},
			want:    []string{"AB", "LOWER"},
			exact:   2,
		},
		{
			name:    "an upper-case glob matches the raw name",
			mode:    ModeFiles,
			prepare: withConfigBlob("FAKE.cfg"),
			files:   []File{sibling, {"FAKE.cfg", []byte("LOWER")}},
			want:    []string{"AB", "LOWER"},
			exact:   2,
		},
		{
			name:    "a character class is matched against the raw name",
			mode:    ModeFiles,
			prepare: withConfigBlob("[A-Z]ake.cfg"),
			files:   []File{sibling, {"Fake.cfg", []byte("LOWER")}},
			want:    []string{"AB", "LOWER"},
			exact:   2,
		},
		{
			name:    "a config blob listed first is isolated too",
			mode:    ModeFiles,
			prepare: withConfigBlob("fake.cfg"),
			files:   []File{configBlob, sibling},
			want:    []string{"lower", "AB"},
			exact:   2,
		},
		{
			name:    "a blob named like a copy_files entry is isolated",
			mode:    ModeFiles,
			prepare: copied,
			files:   []File{sibling, configBlob},
			want:    []string{"AB", "lower"},
			exact:   2,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, testCase.run)
	}
}

func TestIsolationFolded(t *testing.T) {
	configBlob := File{"fake.cfg", []byte("LOWER")}
	sibling := File{"a.go", []byte("Ab")}

	testCases := []formatCase{
		{
			name:    "an upper-case glob isolates a lower-case name",
			mode:    ModeFiles,
			prepare: withConfigBlob("FAKE.cfg"),
			files:   []File{sibling, configBlob},
			want:    []string{"AB", "lower"},
			exact:   2,
		},
		{
			name:    "an upper-case wildcard glob isolates a lower-case name",
			mode:    ModeFiles,
			prepare: withConfigBlob("*.CFG"),
			files:   []File{sibling, configBlob},
			want:    []string{"AB", "lower"},
			exact:   2,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, testCase.run)
	}
}

func TestFold(t *testing.T) {
	testCases := []struct {
		name  string
		left  string
		right string
		same  bool
	}{
		{"ascii case", "Foo.GO", "foo.go", true},
		{"final and medial sigma", "\u03c2", "\u03c3", true},
		{"capital sigma joins them", "\u03a3", "\u03c2", true},
		{"long s and s", "\u017f", "s", true},
		{"micro sign and mu", "\u00b5", "\u03bc", true},
		{"distinct letters stay distinct", "a", "b", false},
		{"an eszett is not ss", "\u00df", "ss", false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := fold(testCase.left) == fold(testCase.right); got != testCase.same {
				t.Errorf(
					"fold(%q) == fold(%q) is %v, want %v",
					testCase.left,
					testCase.right,
					got,
					testCase.same,
				)
			}
		})
	}
}

type budgetCase struct {
	name   string
	budget int
	fixed  int
	sizes  []int
	want   [][]int
}

func (testCase budgetCase) run(t *testing.T) {
	t.Helper()

	jobs := make([]job, len(testCase.sizes))

	for index, size := range testCase.sizes {
		name := fmt.Sprintf("%0*d.go", size, index)
		jobs[index] = job{path: name, clean: name}
	}

	step := tool{argv: []string{strings.Repeat("x", testCase.fixed)}}

	got := (&Engine{batchSize: 100}).chunks(step, jobs, testCase.budget)
	if !slices.EqualFunc(got, testCase.want, slices.Equal) {
		t.Errorf("chunks = %v, want %v", got, testCase.want)
	}
}

func sized(count, size int) []int {
	sizes := make([]int, count)
	for index := range sizes {
		sizes[index] = size
	}

	return sizes
}

func TestChunksArgBudget(t *testing.T) {
	const unixBudget = 30000

	testCases := []budgetCase{
		{
			"short paths stay in one chunk", unixBudget, 0, sized(10, 50),
			[][]int{span(0, 10)},
		},
		{
			"long paths are cut once the argument bytes pass the budget",
			unixBudget, 0, sized(40, 1000),
			[][]int{span(0, 29), span(29, 40)},
		},
		{
			"the fixed arguments share the budget",
			unixBudget, 27000, sized(5, 1000),
			[][]int{{0, 1}, {2, 3}, {4}},
		},
		{
			"one path over the budget runs alone",
			unixBudget, 0,
			[]int{100, 40000, 100},
			[][]int{{0}, {1}, {2}},
		},
		{
			"two paths over the budget never share",
			unixBudget, 0,
			[]int{40000, 40000},
			[][]int{{0}, {1}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, testCase.run)
	}
}

func TestChunksCmdBudget(t *testing.T) {
	const cmdBudget = 7500

	testCases := []budgetCase{
		{
			"a cmd.exe budget cuts the same paths sooner",
			cmdBudget,
			0,
			sized(40, 1000),
			[][]int{
				span(0, 7),
				span(7, 14),
				span(14, 21),
				span(21, 28),
				span(28, 35),
				span(35, 40),
			},
		},
		{
			"a cmd.exe budget is shared with the fixed arguments",
			cmdBudget, 7000, sized(3, 1000),
			[][]int{{0}, {1}, {2}},
		},
		{
			"a cmd.exe budget runs a path past it alone",
			cmdBudget, 0,
			[]int{100, 9000, 100},
			[][]int{{0}, {1}, {2}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, testCase.run)
	}
}

func span(from, to int) []int {
	out := make([]int, 0, to-from)
	for index := from; index < to; index++ {
		out = append(out, index)
	}

	return out
}

func TestChunksArgCost(t *testing.T) {
	const exact = 4 * (95 + 5)

	testCases := []budgetCase{
		{"a command line at the budget fits", exact, 95, sized(3, 92), [][]int{{0, 1, 2}}},
		{"one byte past the budget splits", exact - 1, 95, sized(3, 92), [][]int{{0, 1}, {2}}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, testCase.run)
	}
}
