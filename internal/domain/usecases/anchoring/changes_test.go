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

func TestReadsAfterNames(t *testing.T) {
	t.Parallel()

	other := readsAfterOther
	libImport := []string{`"example.com/p/pkg/lib"`}

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
			name:      "a method placed after is read through its selector",
			candidate: other("method", "h", "Box.Run"),
			preimage:  "H calls Box.Run", at: 3, want: true,
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
			preimage: "H calls lib.Run", imports: libImport, at: 3, want: true,
		},
		{
			name:      "a test package of the unit reads by the directory name",
			candidate: other("func", "pkg/lib#lib_test", "Run"),
			preimage:  "H calls lib.Run", imports: libImport, at: 3, want: true,
		},
	}

	checkReadsAfter(t, testCases)
}

func TestReadsAfterFalseFolds(t *testing.T) {
	t.Parallel()

	other := readsAfterOther
	entitiesImport := []string{`"example.com/p/entities"`}

	testCases := []readsAfterCase{
		{
			name:      "a selector the final text still holds is not a read",
			candidate: other("method", "h", "assertionRule.Name"),
			preimage:  "H uses d.Key.Name", final: "H uses d.Key.Name", at: 3,
		},
		{
			name:      "a selector the fix removed is a read",
			candidate: other("method", "h", "assertionRule.Name"),
			preimage:  "H uses d.Key.Name", final: "H uses d.Key", at: 3, want: true,
		},
		{
			name:      "a selector with no final text counts",
			candidate: other("method", "h", "assertionRule.Name"),
			preimage:  "H uses d.Key.Name", at: 3, want: true,
		},
		{
			name:      "a package qualifier is not a method selector",
			candidate: other("method", "h", "Planner.Plan"),
			preimage:  "p := entities.Plan{}", final: "p := 1", imports: entitiesImport, at: 3,
		},
		{
			name:      "a qualifier that is no package still selects a method",
			candidate: other("method", "h", "Planner.Plan"),
			preimage:  "p := planner.Plan()", final: "p := 1", imports: entitiesImport, at: 3,
			want: true,
		},
		{
			name:      "a package qualifier beside a real selector still reads it",
			candidate: other("method", "h", "Planner.Plan"),
			preimage:  "entities.Plan(x.Plan())", final: "p := 1", imports: entitiesImport, at: 3,
			want: true,
		},
		{
			name:      "a type outside the imports of a plain directory is not read by a qualifier",
			candidate: other("type", "infra/lsp", "Position"),
			preimage:  "p := entities.Position{}", imports: entitiesImport, at: 3,
		},
		{
			name:      "a module root answers to any import",
			candidate: other("func", "aaext", "Run"),
			preimage:  "H calls ext.Run", imports: []string{`ext "example.com/ext"`},
			isModule: true, at: 3, want: true,
		},
		{
			name:      "a plain directory does not answer to a differing import",
			candidate: other("func", "aaext", "Run"),
			preimage:  "H calls ext.Run", imports: []string{`ext "example.com/ext"`}, at: 3,
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
	libImport := []string{`"example.com/p/pkg/lib"`}

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
			preimage: "H calls lib.Run", imports: libImport,
			at: 3, isReading: true, isParsed: true, asks: 1,
			reads: []string{"lib", "lib.Run"}, want: true,
		},
		{
			name: "a qualified read on two lines", candidate: other("func", "pkg/lib", "Run"),
			preimage: "H calls lib.\n\tRun", imports: libImport,
			at: 3, isReading: true, isParsed: true, asks: 1,
			reads: []string{"lib", "lib.Run"}, want: true,
		},
		{
			name: "a bare name is not another unit", candidate: other("func", "pkg/lib", "Run"),
			preimage: "H calls lib.Run", imports: libImport,
			at: 3, isReading: true, isParsed: true, asks: 1, reads: []string{"Run"},
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
	libImport := []string{`"example.com/p/pkg/lib"`}

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
			name:      "a qualified read of it",
			candidate: other("func", "pkg/lib", "\u00c9xito"),
			preimage:  "H calls lib.\u00c9xito", imports: libImport,
			at: 3, isReading: true, isParsed: true, asks: 1,
			reads: []string{"lib", "lib.\u00c9xito"}, want: true,
		},
	}

	checkReadsAfter(t, testCases)
}
