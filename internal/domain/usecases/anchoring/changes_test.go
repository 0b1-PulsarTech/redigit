package anchoring

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestChangedDeclarations(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		oldSrc string
		newSrc string
		want   []changed
		isNew  bool
		born   []entities.DeclKey
	}{
		{
			name:   "a rewritten declaration keeps its parent text",
			oldSrc: "A one\nB two\n", newSrc: "A uno\nB two\n",
			want: []changed{{atAnchor: fkey("A"), preimage: "A one"}},
		},
		{
			name:   "a reformatted declaration is not a change",
			oldSrc: "A one two\n", newSrc: "A  one\ttwo\n",
		},
		{
			name:   "a declaration only gofmt spaced differently is not a change",
			oldSrc: "A x:=1\n", newSrc: "A x := 1\n",
		},
		{
			name:   "a new declaration has no parent text and is born",
			oldSrc: "A one\n", newSrc: "A one\nB two\n",
			born: []entities.DeclKey{fkey("B")},
		},
		{
			name:   "a deleted declaration is not reported",
			oldSrc: "A one\nB two\n", newSrc: "A one\n",
		},
		{
			name:   "every declaration of a file the anchor adds is born",
			newSrc: "A one\nB two\n", isNew: true,
			born: []entities.DeclKey{fkey("A"), fkey("B")},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			blobs := map[string]string{"old": testCase.oldSrc, "new": testCase.newSrc}
			planner := New(Config{Revisions: mapBlobs(gomock.NewController(t), blobs)})
			change := FileChange{Path: "a.go", OldPath: "a.go", Blob: "new", OldBlob: "old"}

			if testCase.isNew {
				change.OldBlob = ""
			}

			got, born, err := planner.changedDecls(context.Background(), linesParser{}, change)
			if err != nil {
				t.Fatalf("changedDecls() = %v", err)
			}

			if !reflect.DeepEqual(got, testCase.want) || !slices.Equal(born, testCase.born) {
				t.Fatalf("changedDecls() = %+v born %v, want %+v born %v",
					got, born, testCase.want, testCase.born)
			}
		})
	}
}

func TestChangedDeclarationsFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")

	testCases := []struct {
		name     string
		failBlob string
	}{
		{name: "the parent blob cannot be read", failBlob: "old"},
		{name: "the anchor blob cannot be read", failBlob: "new"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			revisions := NewMockRevisions(gomock.NewController(t))
			revisions.EXPECT().
				Blob(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, oid string) ([]byte, error) {
					if oid == testCase.failBlob {
						return nil, boom
					}

					return []byte("A one\n"), nil
				}).
				AnyTimes()

			change := FileChange{Path: "a.go", OldPath: "a.go", Blob: "new", OldBlob: "old"}

			_, _, err := New(Config{Revisions: revisions}).
				changedDecls(context.Background(), linesParser{}, change)
			if !errors.Is(err, boom) {
				t.Fatalf("changedDecls() error = %v, want %v", err, boom)
			}
		})
	}
}

type readsAfterCase struct {
	name      string
	candidate entities.Decl
	preimage  string
	at        int
	homeAt    int
	homeFile  string
	homeKind  entities.DeclKind
	reads     []string
	asks      int
	isReading bool
	isParsed  bool
	isMoving  bool
	isMissing bool
	isTested  bool
	want      bool
}

func readsAfterHome() entities.Decl {
	return entities.Decl{
		Key:  entities.DeclKey{File: "h/h.go", Kind: entities.KindFunc, Name: "H"},
		Unit: "h",
	}
}

func readsAfterOther(kind entities.DeclKind, unit entities.UnitID, name string) entities.Decl {
	key := entities.DeclKey{File: "x.go", Kind: kind, Name: name}

	return entities.Decl{Key: key, Unit: unit}
}

func readsAfterTesting(decl entities.Decl) entities.Decl {
	decl.IsTest = true

	return decl
}

func readsAfterHelper(name string) entities.Decl {
	key := entities.DeclKey{File: "h/h_test.go", Kind: entities.KindFunc, Name: name}

	return entities.Decl{Key: key, Unit: "h"}
}

func checkReadsAfter(t *testing.T, testCases []readsAfterCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			home := readsAfterHome()
			home.IsTest = testCase.isTested

			if testCase.homeKind != "" {
				home.Key.Kind = testCase.homeKind
			}

			if testCase.homeFile != "" {
				home.Key.File = testCase.homeFile
			}

			graph := entities.NewGraph([]entities.Decl{home, testCase.candidate}, nil, nil)
			view := scene{graph: graph, decls: graph.Decls()}

			if testCase.isReading {
				reader := NewMockReader(gomock.NewController(t))
				reader.EXPECT().
					Reads(home.Key.File, testCase.preimage).
					Return(testCase.reads, testCase.isParsed).
					Times(testCase.asks)

				view.reader = reader
			}

			index := map[entities.DeclKey]int{home.Key: testCase.homeAt}

			if !testCase.isMissing {
				index[testCase.candidate.Key] = testCase.at
			}

			moving := map[entities.DeclKey]bool{testCase.candidate.Key: testCase.isMoving}
			placed := entry{key: home.Key, preimage: testCase.preimage}

			got := view.readsAfter(placed, index, moving, 1)
			if (got != "") != testCase.want {
				t.Fatalf("readsAfter() = %q, want a hit: %t", got, testCase.want)
			}
		})
	}
}

func TestReadsAfterNames(t *testing.T) {
	t.Parallel()

	other := readsAfterOther

	testCases := []readsAfterCase{
		{
			name: "a function of the same unit placed after", candidate: other("func", "h", "Run"),
			preimage: "H calls Run", at: 3, want: true,
		},
		{
			name: "a type of the same unit placed after", candidate: other("type", "h", "Box"),
			preimage: "H holds Box", at: 3, want: true,
		},
		{
			name: "a value of the same unit placed after", candidate: other("value", "h", "Limit"),
			preimage: "H uses Limit", at: 3, want: true,
		},
		{
			name:      "a method placed after is not scanned",
			candidate: other("method", "h", "Box.Run"),
			preimage:  "H calls Box.Run", at: 3,
		},
		{
			name:      "a statement placed after is not scanned",
			candidate: other("stmt", "h", "Main#1"),
			preimage:  "H calls Main#1", at: 3,
		},
		{
			name: "a name only as part of a longer word", candidate: other("func", "h", "Run"),
			preimage: "H calls Runner", at: 3,
		},
		{
			name: "a bare name of another unit", candidate: other("func", "pkg/lib", "Run"),
			preimage: "H calls Run", at: 3,
		},
		{
			name: "a qualified name of another unit", candidate: other("func", "pkg/lib", "Run"),
			preimage: "H calls lib.Run", at: 3, want: true,
		},
		{
			name:      "a test package of the unit reads by the directory name",
			candidate: other("func", "pkg/lib#lib_test", "Run"),
			preimage:  "H calls lib.Run", at: 3, want: true,
		},
	}

	checkReadsAfter(t, testCases)
}

func TestReadsAfterPlaces(t *testing.T) {
	t.Parallel()

	other := readsAfterOther

	testCases := []readsAfterCase{
		{
			name: "a declaration placed at the activation", candidate: other("func", "h", "Run"),
			preimage: "H calls Run", at: 1,
		},
		{
			name:      "a declaration placed before the activation",
			candidate: other("func", "h", "Run"),
			preimage:  "H calls Run", at: 0,
		},
		{
			name:      "a declaration that moves to the activation",
			candidate: other("func", "h", "Run"),
			preimage:  "H calls Run", at: 3, isMoving: true,
		},
		{
			name: "a declaration that is not placed", candidate: other("func", "h", "Run"),
			preimage: "H calls Run", at: 3, isMissing: true,
		},
		{
			name:      "a document entry reads nothing",
			candidate: other("func", "h", "Run"),
			preimage:  "Run it.", at: 3, homeKind: entities.KindOther,
		},
		{
			name: "the declaration itself", candidate: readsAfterHome(),
			preimage: "H calls H", at: 5, homeAt: 5,
		},
	}

	checkReadsAfter(t, testCases)
}

func TestReadsAfterTestSide(t *testing.T) {
	t.Parallel()

	testCases := []readsAfterCase{
		{
			name:      "a helper in a test file is not read by production code",
			candidate: readsAfterHelper("decl"),
			preimage:  "H ranges decl", at: 3,
		},
		{
			name:      "a test declaration is read by a test",
			candidate: readsAfterTesting(readsAfterOther("func", "h", "decl")),
			preimage:  "H ranges decl", at: 3, isTested: true, want: true,
		},
		{
			name:      "a helper in a test file is read by a test",
			candidate: readsAfterHelper("decl"),
			preimage:  "H ranges decl", at: 3, isTested: true, want: true,
		},
		{
			name:      "a helper in a test file is read from a test file",
			candidate: readsAfterHelper("decl"),
			preimage:  "H ranges decl", at: 3, homeFile: "h/other_test.go", want: true,
		},
	}

	checkReadsAfter(t, testCases)
}

func TestReadsAfterByParsing(t *testing.T) {
	t.Parallel()

	other := readsAfterOther

	testCases := []readsAfterCase{
		{
			name: "lv1 a local of the name", candidate: other("func", "h", "step"),
			preimage: "total := 0; for _, step := range xs { total += step }",
			at:       3, isReading: true, isParsed: true, asks: 1,
		},
		{
			name: "lv2 a parameter of the name", candidate: other("func", "h", "step"),
			preimage: "func H(step []int) int { return len(step) }",
			at:       3, isReading: true, isParsed: true, asks: 1, reads: []string{"int", "len"},
		},
		{
			name: "lv3 a field of the name", candidate: other("func", "h", "step"),
			preimage: "var pair struct{ step int }; return pair.step",
			at:       3, isReading: true, isParsed: true, asks: 1,
		},
		{
			name: "lv4 a comment", candidate: other("func", "h", "step"),
			preimage: "// step comes later",
			at:       3, isReading: true, isParsed: true, asks: 1,
		},
		{
			name: "lv5 a string", candidate: other("func", "h", "step"),
			preimage: "len(\"step\")",
			at:       3, isReading: true, isParsed: true, asks: 1, reads: []string{"len"},
		},
		{
			name: "lv6 a local is not the type of the name", candidate: other("type", "h", "entry"),
			preimage: "entry := 1; return entry",
			at:       3, isReading: true, isParsed: true, asks: 1,
		},
		{
			name: "lv7 a function used as a value", candidate: other("func", "h", "step"),
			preimage: "fn := step; return fn()",
			at:       3, isReading: true, isParsed: true, asks: 1,
			reads: []string{"step"}, want: true,
		},
		{
			name: "lv8 a name used after its loop", candidate: other("func", "h", "step"),
			preimage: "for _, step := range xs {_ = step}; return step()",
			at:       3, isReading: true, isParsed: true, asks: 1, reads: []string{"step", "xs"},
			want: true,
		},
		{
			name: "lv9 a type in a composite literal", candidate: other("type", "h", "kind"),
			preimage: "return kind{n: 1}.n",
			at:       3, isReading: true, isParsed: true, asks: 1,
			reads: []string{"kind"}, want: true,
		},
		{
			name: "a qualified read of another unit", candidate: other("func", "pkg/lib", "Run"),
			preimage: "H calls lib.Run",
			at:       3, isReading: true, isParsed: true, asks: 1,
			reads: []string{"lib", "lib.Run"}, want: true,
		},
		{
			name: "a qualified read on two lines", candidate: other("func", "pkg/lib", "Run"),
			preimage: "H calls lib.\n\tRun",
			at:       3, isReading: true, isParsed: true, asks: 1,
			reads: []string{"lib", "lib.Run"}, want: true,
		},
		{
			name: "a bare name is not another unit", candidate: other("func", "pkg/lib", "Run"),
			preimage: "H calls lib.Run",
			at:       3, isReading: true, isParsed: true, asks: 1, reads: []string{"Run"},
		},
		{
			name: "no word match asks nothing", candidate: other("func", "h", "step"),
			preimage: "H calls Run", at: 3, isReading: true,
		},
	}

	checkReadsAfter(t, testCases)
}

func TestReadsAfterFallsBackToTheWordMatch(t *testing.T) {
	t.Parallel()

	other := readsAfterOther

	testCases := []readsAfterCase{
		{
			name: "lv1 a pre-image that does not parse", candidate: other("func", "h", "step"),
			preimage: "total += step",
			at:       3, isReading: true, asks: 1, want: true,
		},
		{
			name: "lv6 a pre-image that does not parse", candidate: other("type", "h", "entry"),
			preimage: "entry := 1",
			at:       3, isReading: true, asks: 1, want: true,
		},
		{
			name: "lv7 a pre-image that does not parse", candidate: other("func", "h", "step"),
			preimage: "fn := step",
			at:       3, isReading: true, asks: 1, want: true,
		},
		{
			name: "lv9 a pre-image that does not parse", candidate: other("type", "h", "kind"),
			preimage: "return kind{n: 1}.n",
			at:       3, isReading: true, asks: 1, want: true,
		},
		{
			name: "an unparsed pre-image naming nothing", candidate: other("func", "h", "step"),
			preimage: "total += 1", at: 3, isReading: true,
		},
		{
			name: "lv1 with no port wired", candidate: other("func", "h", "step"),
			preimage: "total += step", at: 3, want: true,
		},
		{
			name: "lv7 with no port wired", candidate: other("func", "h", "step"),
			preimage: "fn := step", at: 3, want: true,
		},
		{
			name: "a name with a non-ASCII initial", candidate: other("func", "h", "\u00c9xito"),
			preimage: "total += \u00c9xito", at: 3, isReading: true, asks: 1, want: true,
		},
		{
			name: "a longer word around that initial", candidate: other("func", "h", "\u00c9xito"),
			preimage: "total += \u00c9xitos + a\u00c9xito", at: 3,
		},
		{
			name: "a non-ASCII letter ending a longer word", candidate: other("func", "h", "step"),
			preimage: "total += step\u00e3", at: 3,
		},
		{
			name: "a qualified read of it", candidate: other("func", "pkg/lib", "\u00c9xito"),
			preimage: "H calls lib.\u00c9xito", at: 3, isReading: true, isParsed: true, asks: 1,
			reads: []string{"lib", "lib.\u00c9xito"}, want: true,
		},
	}

	checkReadsAfter(t, testCases)
}
