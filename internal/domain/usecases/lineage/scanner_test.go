package lineage

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"maps"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

var (
	errBoom      = errors.New("boom")
	errBadSource = errors.New("bad source")
)

type fakeLog struct {
	revisions []Revision
	failAfter map[string]int
	failWith  error
	changeErr error
	mutex     sync.Mutex
	reads     map[string]int
}

func (log *fakeLog) Changes(context.Context) iter.Seq2[Revision, error] {
	return func(yield func(Revision, error) bool) {
		for _, revision := range log.revisions {
			if !yield(revision, nil) {
				return
			}
		}

		if log.changeErr != nil {
			yield(Revision{}, log.changeErr)
		}
	}
}

func (log *fakeLog) ReadBlob(_ context.Context, oid string) ([]byte, error) {
	log.mutex.Lock()
	defer log.mutex.Unlock()

	if log.reads == nil {
		log.reads = map[string]int{}
	}

	log.reads[oid]++

	if limit, ok := log.failAfter[oid]; ok && log.reads[oid] > limit {
		return nil, log.failWith
	}

	switch oid {
	case "missing":
		return nil, fmt.Errorf("read: %w", ErrBlobMissing)
	case "boom":
		return nil, errBoom
	}

	return []byte(oid), nil
}

type fakeSyntax struct{}

func (fakeSyntax) Declarations(
	_ context.Context, path string, src []byte,
) ([]entities.Decl, error) {
	if string(src) == "bad" {
		return nil, errBadSource
	}

	kind := entities.KindFunc
	if strings.HasPrefix(path, "db/") {
		kind = entities.KindType
	}

	var out []entities.Decl

	offset := 0

	for line := range strings.SplitSeq(string(src), "\n") {
		if name, _, _ := strings.Cut(line, " "); name != "" {
			out = append(out, entities.Decl{
				Key:  entities.DeclKey{File: path, Kind: kind, Name: name},
				Span: entities.Span{Start: offset, End: offset + len(line)},
			})
		}

		offset += len(line) + 1
	}

	return out, nil
}

func (fakeSyntax) Render(context.Context, []byte, []entities.DeclKey) ([]byte, error) {
	return nil, nil
}

type birthsLog struct{ *fakeLog }

func (log birthsLog) Changes(ctx context.Context) iter.Seq2[indexing.RevisionChange, error] {
	return func(yield func(indexing.RevisionChange, error) bool) {
		for revision, err := range log.fakeLog.Changes(ctx) {
			change := indexing.RevisionChange{Rev: revision.Rev}
			for _, one := range revision.Changes {
				change.Files = append(change.Files, indexing.ChangedFile{
					Path: one.Path, Blob: one.Blob, IsDeleted: one.Status == StatusDeleted,
				})
			}

			if !yield(change, err) {
				return
			}
		}
	}
}

func (log birthsLog) ReadBlob(ctx context.Context, oid string) ([]byte, error) {
	out, err := log.fakeLog.ReadBlob(ctx, oid)
	if errors.Is(err, ErrBlobMissing) {
		return nil, fmt.Errorf("read: %w", indexing.ErrBlobMissing)
	}

	return out, err
}

func add(path, content string) Change {
	return Change{Path: path, Blob: content, Status: 'A'}
}

func modify(path, content string) Change {
	return Change{Path: path, Blob: content, Status: 'M'}
}

func remove(path string) Change {
	return Change{Path: path, Status: StatusDeleted}
}

func renamed(from, fromContent, to, toContent string, similarity int) Change {
	return Change{
		Path: to, OldPath: from, Blob: toContent, OldBlob: fromContent,
		Status: StatusRenamed, Similarity: similarity,
	}
}

func revs(revisions ...Revision) []Revision { return revisions }

func rev(name string, changes ...Change) Revision { return Revision{Rev: name, Changes: changes} }

func snapshotOf(revisions []Revision, skip ...string) entities.Snapshot {
	var snap entities.Snapshot

	for idx, revision := range revisions {
		if !slices.Contains(skip, revision.Rev) {
			snap.History = append(snap.History, entities.SourceTouch{
				Position: entities.Position(idx), Rev: revision.Rev,
			})
		}
	}

	return snap
}

func positions(result Result) map[entities.DeclKey]entities.Position {
	out := make(map[entities.DeclKey]entities.Position, len(result.Births))
	for key, birth := range result.Births {
		out[key] = birth.At
	}

	return out
}

type offCase struct {
	name      string
	revisions []Revision
	skip      []string
}

func offCases() []offCase {
	const moved = "A one two three four"

	return []offCase{
		{"a file that gains declarations", revs(
			rev("r0", add("a.go", "A one")),
			rev("r1", modify("a.go", "A one\nB two\nC three")),
		), nil},
		{"delete then re-create keeps the earliest", revs(
			rev("r0", add("a.go", "A one")),
			rev("r1", remove("a.go")),
			rev("r2", add("a.go", "A one\nB two")),
		), nil},
		{"a rename is an add", revs(
			rev("r0", add("a.go", "A one two three four")),
			rev("r1", renamed("a.go", "A one two three four", "b.go", "A one two three four", 100)),
		), nil},
		{"a copy is an add", revs(
			rev("r0", add("a.go", "A one")),
			rev("r1", Change{Path: "b.go", OldPath: "a.go", Blob: "A one", Status: 'C'}),
		), nil},
		{"one blob under two paths", revs(
			rev("r0", add("a.go", "A one"), add("b.go", "A one")),
		), nil},
		{"one blob under two paths a parser tells apart", revs(
			rev("r0", add("a.go", "A one"), add("db/b.go", "A one")),
		), nil},
		{"a rename into a path another chunker claims", revs(
			rev("r0", add("a.go", moved)),
			rev("r1", renamed("a.go", moved, "db/a.go", moved, 100)),
		), nil},
		{"a blob nobody can read", revs(
			rev("r0", add("a.go", "missing"), add("b.go", "A one")),
		), nil},
		{"a blob that does not parse", revs(
			rev("r0", add("a.go", "bad"), add("b.go", "A one")),
		), nil},
		{"files no language owns", revs(
			rev("r0", add("notes.txt", "A one"), add("a.go", "B two")),
		), nil},
		{"a revision outside the snapshot", revs(
			rev("r0", add("a.go", "A one")),
			rev("r1", add("b.go", "B two")),
			rev("r2", add("c.go", "C three")),
		), []string{"r1"}},
		{"nothing changed", revs(rev("r0")), nil},
	}
}

func TestScanFollowOffEqualsBirths(t *testing.T) {
	t.Parallel()

	testCases := offCases()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			snap := snapshotOf(testCase.revisions, testCase.skip...)
			log := &fakeLog{revisions: testCase.revisions}

			want, err := indexing.NewScanner(birthsLog{log}, indexing.Config{
				Concurrency: 2,
				Languages: []indexing.Language{
					{Extensions: []string{".go"}, Syntax: fakeSyntax{}},
				},
			}).Scan(t.Context(), snap)
			if err != nil {
				t.Fatalf("births = %v", err)
			}

			got, err := NewScanner(&fakeLog{revisions: testCase.revisions}, config()).
				Scan(t.Context(), snap)
			if err != nil {
				t.Fatalf("Scan() = %v", err)
			}

			if !reflect.DeepEqual(positions(got), want.At) {
				t.Errorf("births differ:\n got %v\nwant %v", positions(got), want.At)
			}

			if !reflect.DeepEqual(got.Unreadable, want.Unreadable) {
				t.Errorf("unreadable differ:\n got %v\nwant %v", got.Unreadable, want.Unreadable)
			}

			for key, birth := range got.Births {
				if len(birth.Via) != 0 {
					t.Errorf("%v carries Via with follow off: %v", key, birth.Via)
				}
			}
		})
	}
}

func TestScanFailures(t *testing.T) {
	t.Parallel()

	one := revs(rev("r0", add("a.go", "A one")))
	snap := snapshotOf(one)

	testCases := []struct {
		name    string
		log     *fakeLog
		snap    entities.Snapshot
		wantErr error
	}{
		{"no history", &fakeLog{revisions: one}, entities.Snapshot{}, errNoHistory},
		{
			"the log fails", &fakeLog{revisions: one, changeErr: errBoom}, snap, errBoom,
		},
		{
			"a blob read fails", &fakeLog{revisions: revs(rev("r0", add("a.go", "boom")))},
			snap, errBoom,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewScanner(testCase.log, config()).Scan(t.Context(), testCase.snap)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Scan() = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestScanCancelled(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	ctx, cancel := context.WithCancel(t.Context())

	log := NewMockRevisionLog(ctrl)
	log.EXPECT().Changes(gomock.Any()).Return(
		(&fakeLog{revisions: revs(rev("r0", add("a.go", "A")))}).Changes(ctx),
	)
	log.EXPECT().ReadBlob(gomock.Any(), "A").DoAndReturn(
		func(context.Context, string) ([]byte, error) {
			cancel()

			return nil, context.Canceled
		},
	)

	_, err := NewScanner(log, config()).Scan(ctx, snapshotOf(revs(rev("r0"))))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Scan() = %v, want context.Canceled", err)
	}
}

func TestScanParserCancelled(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	ctx, cancel := context.WithCancel(t.Context())

	log := NewMockRevisionLog(ctrl)
	log.EXPECT().Changes(gomock.Any()).Return(
		(&fakeLog{revisions: revs(rev("r0", add("a.go", "A")))}).Changes(ctx),
	)
	log.EXPECT().ReadBlob(gomock.Any(), "A").Return([]byte("A"), nil)

	parser := NewMockParser(ctrl)
	parser.EXPECT().Declarations(gomock.Any(), "a.go", []byte("A")).DoAndReturn(
		func(context.Context, string, []byte) ([]entities.Decl, error) {
			cancel()

			return nil, errBadSource
		},
	)

	cfg := Config{Languages: []Language{{Extensions: []string{".go"}, Parser: parser}}}

	_, err := NewScanner(log, cfg).Scan(ctx, snapshotOf(revs(rev("r0"))))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Scan() = %v, want context.Canceled", err)
	}
}

func TestScanTwiceIsEqual(t *testing.T) {
	t.Parallel()

	revisions := revs(
		rev("r0", add("a.go", "A one\nB two\nC three")),
		rev("r1", renamed("a.go", "A one\nB two\nC three", "b.go", "A one\nB two\nC three", 100)),
	)

	runs := make([]map[entities.DeclKey]Birth, 0, 5)

	for range 5 {
		got, err := NewScanner(&fakeLog{revisions: revisions}, followConfig()).
			Scan(t.Context(), snapshotOf(revisions))
		if err != nil {
			t.Fatalf("Scan() = %v", err)
		}

		runs = append(runs, got.Births)
	}

	for _, run := range runs[1:] {
		if !maps.EqualFunc(run, runs[0], func(left, right Birth) bool {
			return reflect.DeepEqual(left, right)
		}) {
			t.Fatalf("runs differ: %v vs %v", run, runs[0])
		}
	}
}

func config() Config {
	return Config{
		Concurrency: 2,
		Languages:   []Language{{Extensions: []string{".go"}, Parser: fakeSyntax{}}},
	}
}

func followConfig() Config {
	cfg := config()
	cfg.Follow = true

	return cfg
}
