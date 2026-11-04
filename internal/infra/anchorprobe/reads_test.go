package anchorprobe

import (
	"slices"
	"testing"
)

type readsCase struct {
	name     string
	file     string
	text     string
	present  []string
	absent   []string
	isParsed bool
}

func checkReads(t *testing.T, testCases []readsCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, isParsed := (&Probe{}).Reads(testCase.file, testCase.text)
			if isParsed != testCase.isParsed {
				t.Fatalf("Reads() parsed = %t, want %t", isParsed, testCase.isParsed)
			}

			for _, name := range testCase.present {
				if !slices.Contains(got, name) {
					t.Errorf("Reads() = %v, want it to hold %q", got, name)
				}
			}

			for _, name := range testCase.absent {
				if slices.Contains(got, name) {
					t.Errorf("Reads() = %v, want it to lack %q", got, name)
				}
			}

			if !isParsed && got != nil {
				t.Errorf("Reads() = %v, want nil when it did not parse", got)
			}
		})
	}
}

func TestReadsLeavesWhatIsNotAReference(t *testing.T) {
	t.Parallel()

	testCases := []readsCase{
		{
			name: "lv1 a local of the name", file: "h.go", isParsed: true,
			text: "func H(xs []int) int {\n" +
				"\ttotal := 0\n\tfor _, step := range xs { total += step }\n\treturn total\n}",
			absent: []string{"step", "total"},
		},
		{
			name: "lv2 a parameter of the name", file: "h.go", isParsed: true,
			text:   "func H(step []int) int { return len(step) }",
			absent: []string{"step"},
		},
		{
			name: "lv3 a field of the name", file: "h.go", isParsed: true,
			text:   "func H() int {\n\tvar pair struct{ step int }\n\treturn pair.step\n}",
			absent: []string{"step", "pair.step"},
		},
		{
			name: "lv4 a comment", file: "h.go", isParsed: true,
			text:   "// step comes later\nfunc H() int { return 1 }",
			absent: []string{"step"},
		},
		{
			name: "lv5 a string", file: "h.go", isParsed: true,
			text:   "func H() int { return len(\"step\") }",
			absent: []string{"step"},
		},
		{
			name: "lv6 a local named like a type", file: "h.go", isParsed: true,
			text:   "func H() int {\n\tentry := 1\n\treturn entry\n}",
			absent: []string{"entry"},
		},
		{
			name: "a field of a local next to a qualifier", file: "h.go", isParsed: true,
			text:    "func H(cfg Config) int { return cfg.Size + lib.Run() }",
			present: []string{"Config", "lib.Run"}, absent: []string{"cfg.Size", "Size"},
		},
		{
			name: "a key that is a local of the pre-image", file: "h.go", isParsed: true,
			text:   "func H() int {\n\tlimit := 1\n\treturn len(map[int]int{limit: 1})\n}",
			absent: []string{"limit"},
		},
	}

	checkReads(t, testCases)
}

func TestReadsKeepsEveryReference(t *testing.T) {
	t.Parallel()

	testCases := []readsCase{
		{
			name: "lv7 a function used as a value", file: "h.go", isParsed: true,
			text:    "func H() int {\n\tfn := step\n\treturn fn()\n}",
			present: []string{"step"}, absent: []string{"fn"},
		},
		{
			name: "lv8 a name used after its loop", file: "h.go", isParsed: true,
			text: "func H(xs []int) int {\n" +
				"\tfor _, step := range xs { _ = step }\n\treturn step()\n}",
			present: []string{"step"}, absent: []string{"xs"},
		},
		{
			name: "lv9 a type in a composite literal", file: "h.go", isParsed: true,
			text:    "func H() int { return kind{n: 1}.n }",
			present: []string{"kind"},
		},
		{
			name: "a method pre-image", file: "h.go", isParsed: true,
			text:    "func (box *Box) Run() int { return step() }",
			present: []string{"Box", "step"}, absent: []string{"Run", "box"},
		},
		{
			name: "a type declaration", file: "h.go", isParsed: true,
			text:    "type Pair struct {\n\tleft  Box\n\tright *Other\n}",
			present: []string{"Box", "Other"}, absent: []string{"left", "right"},
		},
		{
			name: "a const group", file: "h.go", isParsed: true,
			text:    "const (\n\tA = limit\n\tB = A + 1\n)",
			present: []string{"limit"}, absent: []string{"A", "B"},
		},
		{
			name: "a var group", file: "h.go", isParsed: true,
			text:    "var (\n\tone = build()\n\ttwo = one\n)",
			present: []string{"build"}, absent: []string{"one", "two"},
		},
		{
			name: "a generic function", file: "h.go", isParsed: true,
			text:    "func Map[T any](xs []T) []T { return apply(xs) }",
			present: []string{"apply"}, absent: []string{"T", "xs"},
		},
		{
			name: "a package qualifier that is not resolved", file: "h.go", isParsed: true,
			text:    "func H() int { return lib.Run() }",
			present: []string{"lib", "lib.Run"},
		},
		{
			name: "a name a dot import would bring in", file: "h.go", isParsed: true,
			text:    "func H() int { return Run() }",
			present: []string{"Run"},
		},
		{
			name: "a key of a map literal", file: "h.go", isParsed: true,
			text:    "func H() int { return len(map[int]int{limit: 1}) }",
			present: []string{"limit"},
		},
		{
			name: "a key of a slice literal", file: "h.go", isParsed: true,
			text:    "func H() int { return len([]int{limit: 1}) }",
			present: []string{"limit"},
		},
		{
			name: "a key of a named literal, which may be a field", file: "h.go", isParsed: true,
			text:    "func H() int { return Pair{step: 1}.step }",
			present: []string{"Pair", "step"},
		},
	}

	checkReads(t, testCases)
}

func TestReadsDeclinesWhatItCannotParse(t *testing.T) {
	t.Parallel()

	testCases := []readsCase{
		{name: "a statement of a cut function", file: "h.go", text: "total += step"},
		{name: "a return statement", file: "h.go", text: "return step()"},
		{name: "text that does not parse", file: "h.go", text: "func H( {"},
		{name: "a file that is not Go", file: "README.md", text: "func H() int { return 1 }"},
		{name: "a file without a name", text: "func H() int { return step() }"},
	}

	checkReads(t, testCases)
}

func TestReadsHoldsEverySelectorOfAnExpression(t *testing.T) {
	t.Parallel()

	testCases := []readsCase{
		{
			name: "a selector on a resolved local", file: "h.go", isParsed: true,
			text:    "func H(t *T) int { return t.M() }",
			present: []string{".M"}, absent: []string{"t.M", "M"},
		},
		{
			name: "a selector on a field of a local", file: "h.go", isParsed: true,
			text:    "func H(s *S) int { return s.bbb.M() }",
			present: []string{".M", ".bbb"}, absent: []string{"bbb.M", "s.bbb"},
		},
		{
			name: "a selector on a selector of a package", file: "h.go", isParsed: true,
			text:    "func H() int { return lib.Var.M() }",
			present: []string{".M", "lib.Var"}, absent: []string{"lib.M"},
		},
		{
			name: "a selector on a call", file: "h.go", isParsed: true,
			text:    "func H() int { return build().M() }",
			present: []string{".M", "build"},
		},
		{
			name: "a selector on an index", file: "h.go", isParsed: true,
			text:    "func H(xs []T) int { return xs[0].M() }",
			present: []string{".M"},
		},
		{
			name: "a selector on a literal", file: "h.go", isParsed: true,
			text:    "func H() int { return lib.U{}.M() }",
			present: []string{".M", "lib.U"},
		},
		{
			name: "a selector split over two lines", file: "h.go", isParsed: true,
			text:    "func H(t *T) int {\n\treturn t.\n\t\tM()\n}",
			present: []string{".M"},
		},
		{
			name: "a selector on a free name keeps its qualifier", file: "h.go", isParsed: true,
			text:    "func H() int { return pkgvar.M() }",
			present: []string{"pkgvar.M"}, absent: []string{".M"},
		},
		{
			name: "a selector named only in a comment", file: "h.go", isParsed: true,
			text:   "// t.M is gone\nfunc H(t *T) int { return 1 }",
			absent: []string{".M", "t.M"},
		},
		{
			name: "a selector named only in a string", file: "h.go", isParsed: true,
			text:   "func H() string { return \"t.M\" }",
			absent: []string{".M", "t.M"},
		},
	}

	checkReads(t, testCases)
}
