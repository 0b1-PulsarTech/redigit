package anchoring

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/lineage"
)

type deltaCase struct {
	name      string
	oldPath   string
	oldSrc    string
	newSrc    string
	finalPath string
	finalSrc  string
	renames   lineage.Renames
	sliced    string
	want      []entry
	wantSkips []skip
}

func runDelta(t *testing.T, testCase deltaCase) {
	t.Helper()

	ctrl := gomock.NewController(t)
	finalPath := testCase.finalPath

	if finalPath == "" {
		finalPath = "a.go"
	}

	files := map[string]string{finalPath: testCase.finalSrc}
	change := FileChange{Path: "a.go", OldPath: "a.go", Blob: "new", OldBlob: "old"}

	if testCase.oldPath != "" {
		change.OldPath = testCase.oldPath
	}

	blobs := map[string]string{"old": testCase.oldSrc, "new": testCase.newSrc}
	planner := New(Config{
		Languages: goLanguages(),
		Revisions: mapBlobs(ctrl, blobs),
		Finals:    mapFinals(ctrl, files),
		Renames:   testCase.renames,
	})

	graph := graphOf(files)

	if testCase.sliced != "" {
		name := testCase.sliced + "#1"
		key := entities.DeclKey{File: finalPath, Kind: entities.KindStmt, Name: name}
		cut := entities.Decl{Key: key}
		graph = entities.NewGraph(append(graph.Decls(), cut), nil, nil)
	}

	got, err := planner.delta(
		context.Background(),
		graph,
		Anchor{Rev: "fix", Position: 5},
		Revision{Files: []FileChange{change}},
	)
	if err != nil {
		t.Fatalf("delta() = %v", err)
	}

	if !reflect.DeepEqual(got.entries, testCase.want) {
		t.Fatalf("entries = %+v, want %+v", got.entries, testCase.want)
	}

	if !reflect.DeepEqual(got.skipped, testCase.wantSkips) {
		t.Fatalf("skipped = %+v, want %+v", got.skipped, testCase.wantSkips)
	}
}

func TestDeltaChangedDeclarations(t *testing.T) {
	t.Parallel()

	testCases := []deltaCase{
		{
			name:   "a changed declaration carries its parent text",
			oldSrc: "A one\nB two\n", newSrc: "A uno\nB two\n", finalSrc: "A last\nB two\n",
			want: []entry{{
				key: fkey("A"), preimage: "A one", span: entities.Span{End: 6},
			}},
		},
		{
			name:   "a declaration new at the anchor has no pre-image",
			oldSrc: "A one\n", newSrc: "A one\nB two\n", finalSrc: "A one\nB two\n",
		},
		{
			name:   "a declaration deleted at the anchor is ignored",
			oldSrc: "A one\nB two\n", newSrc: "A one\n", finalSrc: "A one\n",
		},
		{
			name:   "only the changed declaration of a file is reported",
			oldSrc: "A one\nB two\n", newSrc: "A one\nB three\n", finalSrc: "A one\nB three\n",
			want: []entry{{
				key: fkey("B"), preimage: "B two", span: entities.Span{Start: 6, End: 13},
			}},
		},
		{
			name:    "a file renamed at the anchor keys the declaration by its new path",
			oldPath: "old.go",
			oldSrc:  "A one\n", newSrc: "A uno\n", finalSrc: "A last\n",
			want: []entry{{
				key: fkey("A"), preimage: "A one", span: entities.Span{End: 6},
			}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			runDelta(t, testCase)
		})
	}
}

func TestDeltaFinalKeys(t *testing.T) {
	t.Parallel()

	renamed := lineage.Renames{fkey("A"): {{At: 9, From: fkey("A"), To: fkey("Z")}}}
	early := lineage.Renames{fkey("A"): {{At: 2, From: fkey("A"), To: fkey("Z")}}}
	target := entities.DeclKey{File: "z.go", Kind: entities.KindFunc, Name: "A"}
	moved := lineage.Renames{fkey("A"): {{At: 9, From: fkey("A"), To: target}}}

	testCases := []deltaCase{
		{
			name:   "a declaration renamed after the anchor is reported, not spliced",
			oldSrc: "A one\n", newSrc: "A uno\n", finalSrc: "Z last\n", renames: renamed,
			wantSkips: []skip{{key: fkey("Z"), reason: reasonRenamed}},
		},
		{
			name:   "a rename before the anchor is already in its key",
			oldSrc: "A one\n", newSrc: "A uno\n", finalSrc: "A last\n", renames: early,
			want: []entry{{
				key: fkey("A"), preimage: "A one", span: entities.Span{End: 6},
			}},
		},
		{
			name:   "a file moved after the anchor keeps the entry under the final key",
			oldSrc: "A one\n", newSrc: "A uno\n", finalPath: "z.go", finalSrc: "A last\n",
			renames: moved,
			want:    []entry{{key: target, preimage: "A one", span: entities.Span{End: 6}}},
		},
		{
			name:   "a function the final tree cuts into statements",
			oldSrc: "A one\n", newSrc: "A uno\n", finalSrc: "A last\n", sliced: "A",
			wantSkips: []skip{{key: fkey("A"), reason: "cut into statements"}},
		},
		{
			name:   "a parent blob that does not parse skips the file",
			oldSrc: "BROKEN\n", newSrc: "A uno\n", finalSrc: "A last\n",
			wantSkips: []skip{{key: entities.DeclKey{File: "a.go"}, reason: reasonUnparsed}},
		},
		{
			name:   "an anchor blob that does not parse skips the file",
			oldSrc: "A one\n", newSrc: "BROKEN\n", finalSrc: "A last\n",
			wantSkips: []skip{{key: entities.DeclKey{File: "a.go"}, reason: reasonUnparsed}},
		},
		{
			name:   "a declaration the final tree no longer holds",
			oldSrc: "A one\n", newSrc: "A uno\n", finalSrc: "B other\n",
			wantSkips: []skip{{key: fkey("A"), reason: reasonGone}},
		},
		{
			name:   "a declaration whose final text is its pre-image again",
			oldSrc: "A one\n", newSrc: "A uno\n", finalSrc: "A one\n",
			wantSkips: []skip{{key: fkey("A"), reason: reasonReverted}},
		},
		{
			name:   "a final text that is its pre-image gofmt spaced is reverted",
			oldSrc: "A x:=1\n", newSrc: "A uno\n", finalSrc: "A x := 1\n",
			wantSkips: []skip{{key: fkey("A"), reason: reasonReverted}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			runDelta(t, testCase)
		})
	}
}

func TestDeltaFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")

	testCases := []struct {
		name      string
		failBlob  string
		failFinal bool
		final     string
		wantErr   error
	}{
		{name: "the parent blob cannot be read", failBlob: "old", final: "A last\n", wantErr: boom},
		{name: "the anchor blob cannot be read", failBlob: "new", final: "A last\n", wantErr: boom},
		{name: "the final file cannot be read", failFinal: true, final: "A last\n", wantErr: boom},
		{name: "a span reaching past its final file", wantErr: ErrAnchor},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			revisions := NewMockRevisions(ctrl)
			revisions.EXPECT().
				Blob(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, oid string) ([]byte, error) {
					if oid == testCase.failBlob {
						return nil, boom
					}

					blobs := map[string]string{"old": "A one\n", "new": "A uno\n"}

					return []byte(blobs[oid]), nil
				}).
				AnyTimes()

			finals := NewMockFinals(ctrl)
			finals.EXPECT().
				Read(gomock.Any(), gomock.Any()).
				DoAndReturn(func(context.Context, string) ([]byte, error) {
					if testCase.failFinal {
						return nil, boom
					}

					return []byte(testCase.final), nil
				}).
				AnyTimes()

			planner := New(Config{Languages: goLanguages(), Revisions: revisions, Finals: finals})
			change := FileChange{Path: "a.go", OldPath: "a.go", Blob: "new", OldBlob: "old"}
			graph := graphOf(map[string]string{"a.go": "A last\n"})

			revision := Revision{Files: []FileChange{change}}

			_, err := planner.delta(context.Background(), graph, Anchor{Position: 5}, revision)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("delta() error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}
