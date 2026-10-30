package lineage

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func key(file, name string) entities.DeclKey {
	return entities.DeclKey{File: file, Kind: entities.KindFunc, Name: name}
}

func step(at entities.Position, from, to entities.DeclKey, fileScore int) Rename {
	return Rename{At: at, From: from, To: to, FileScore: fileScore}
}

func textStep(at entities.Position, from, to entities.DeclKey, fileScore, textScore int) Rename {
	return Rename{At: at, From: from, To: to, FileScore: fileScore, TextScore: textScore}
}

const (
	tailWords = "one two three four five six"
)

type followCase struct {
	name      string
	revisions []Revision
	skip      []string
	want      map[entities.DeclKey]Birth
}

func renameCases() []followCase {
	twoDecls := "A alpha beta gamma delta epsilon\nB zeta eta theta iota kappa"

	return []followCase{
		{
			name: "a file rename with the same names",
			revisions: revs(
				rev("r0", add("a.go", twoDecls)),
				rev("r1", renamed("a.go", twoDecls, "b.go", twoDecls, 95)),
			),
			want: map[entities.DeclKey]Birth{
				key("a.go", "A"): {At: 0},
				key("a.go", "B"): {At: 0},
				key("b.go", "A"): {At: 0, Via: []Rename{
					step(1, key("a.go", "A"), key("b.go", "A"), 95),
				}},
				key("b.go", "B"): {At: 0, Via: []Rename{
					step(1, key("a.go", "B"), key("b.go", "B"), 95),
				}},
			},
		},
		{
			name: "a rename plus a declaration renamed by text",
			revisions: revs(
				rev("r0", add("a.go", "Old "+tailWords+"\nKeep "+tailWords+" x")),
				rev("r1", renamed(
					"a.go", "Old "+tailWords+"\nKeep "+tailWords+" x",
					"b.go", "New "+tailWords+"\nKeep "+tailWords+" x", 80,
				)),
			),
			want: map[entities.DeclKey]Birth{
				key("a.go", "Old"):  {At: 0},
				key("a.go", "Keep"): {At: 0},
				key("b.go", "Keep"): {At: 0, Via: []Rename{
					step(1, key("a.go", "Keep"), key("b.go", "Keep"), 80),
				}},
				key("b.go", "New"): {At: 0, Via: []Rename{
					textStep(1, key("a.go", "Old"), key("b.go", "New"), 80, 66),
				}},
			},
		},
		{
			name: "below the threshold the declaration is born anew",
			revisions: revs(
				rev("r0", add("a.go", "Old "+tailWords)),
				rev("r1", renamed(
					"a.go", "Old "+tailWords, "b.go", "New nothing alike at all here", 50,
				)),
			),
			want: map[entities.DeclKey]Birth{
				key("a.go", "Old"): {At: 0},
				key("b.go", "New"): {At: 1},
			},
		},
		{
			name: "two hops chain through Via",
			revisions: revs(
				rev("r0", add("a.go", "A "+tailWords)),
				rev("r1", renamed("a.go", "A "+tailWords, "b.go", "A "+tailWords, 100)),
				rev("r2", renamed("b.go", "A "+tailWords, "c.go", "A "+tailWords, 100)),
			),
			want: map[entities.DeclKey]Birth{
				key("a.go", "A"): {At: 0},
				key("b.go", "A"): {At: 0, Via: []Rename{
					step(1, key("a.go", "A"), key("b.go", "A"), 100),
				}},
				key("c.go", "A"): {At: 0, Via: []Rename{
					step(1, key("a.go", "A"), key("b.go", "A"), 100),
					step(2, key("b.go", "A"), key("c.go", "A"), 100),
				}},
			},
		},
	}
}

func tieCases() []followCase {
	grown := "X " + tailWords + " seven"
	pair := "P " + tailWords + " seven\nQ " + tailWords + " seven"

	return []followCase{
		{
			name: "equal candidates pair by key order",
			revisions: revs(
				rev("r0", add("a.go", grown)),
				rev("r1", modify("a.go", grown+"\nY "+tailWords+" seven")),
				rev("r2", renamed("a.go", grown+"\nY "+tailWords+" seven", "b.go", pair, 90)),
			),
			want: map[entities.DeclKey]Birth{
				key("a.go", "X"): {At: 0},
				key("a.go", "Y"): {At: 1},
				key("b.go", "P"): {At: 0, Via: []Rename{
					textStep(2, key("a.go", "X"), key("b.go", "P"), 90, 71),
				}},
				key("b.go", "Q"): {At: 1, Via: []Rename{
					textStep(2, key("a.go", "Y"), key("b.go", "Q"), 90, 71),
				}},
			},
		},
	}
}

func edgeCases() []followCase {
	return []followCase{
		{
			name: "a copy is an add",
			revisions: revs(
				rev("r0", add("a.go", "A "+tailWords)),
				rev("r1", Change{
					Path: "b.go", OldPath: "a.go", Blob: "A " + tailWords,
					OldBlob: "A " + tailWords, Status: 'C', Similarity: 100,
				}),
			),
			want: map[entities.DeclKey]Birth{
				key("a.go", "A"): {At: 0},
				key("b.go", "A"): {At: 1},
			},
		},
		{
			name: "delete then re-add then rename keeps the first birth",
			revisions: revs(
				rev("r0", add("a.go", "A "+tailWords)),
				rev("r1", remove("a.go")),
				rev("r2", add("a.go", "A "+tailWords)),
				rev("r3", renamed("a.go", "A "+tailWords, "b.go", "A "+tailWords, 100)),
			),
			want: map[entities.DeclKey]Birth{
				key("a.go", "A"): {At: 0},
				key("b.go", "A"): {At: 0, Via: []Rename{
					step(3, key("a.go", "A"), key("b.go", "A"), 100),
				}},
			},
		},
		{
			name: "a rename from a file no language owns is an add",
			revisions: revs(
				rev("r0", add("a.txt", "A "+tailWords)),
				rev("r1", renamed("a.txt", "A "+tailWords, "b.go", "A "+tailWords, 100)),
			),
			want: map[entities.DeclKey]Birth{key("b.go", "A"): {At: 1}},
		},
		{
			name: "a rename that names no old blob is an add",
			revisions: revs(
				rev("r0", add("a.go", "A "+tailWords)),
				rev("r1", renamed("a.go", "", "b.go", "A "+tailWords, 100)),
			),
			want: map[entities.DeclKey]Birth{
				key("a.go", "A"): {At: 0},
				key("b.go", "A"): {At: 1},
			},
		},
		{
			name: "an old file that does not parse gives nothing to inherit",
			revisions: revs(
				rev("r0", add("a.go", "bad")),
				rev("r1", renamed("a.go", "bad", "b.go", "A "+tailWords, 100)),
			),
			want: map[entities.DeclKey]Birth{key("b.go", "A"): {At: 1}},
		},
		{
			name: "a rename in a revision outside the snapshot is ignored",
			revisions: revs(
				rev("r0", add("a.go", "A "+tailWords)),
				rev("r1", renamed("a.go", "A "+tailWords, "b.go", "A "+tailWords, 100)),
			),
			skip: []string{"r1"},
			want: map[entities.DeclKey]Birth{key("a.go", "A"): {At: 0}},
		},
		{
			name: "a renamed file whose origin predates the snapshot starts at the rename",
			revisions: revs(
				rev("r0", add("a.go", "A "+tailWords)),
				rev("r1", renamed("a.go", "A "+tailWords, "b.go", "A "+tailWords, 100)),
			),
			skip: []string{"r0"},
			want: map[entities.DeclKey]Birth{key("b.go", "A"): {At: 1}},
		},
	}
}

func TestScanFollowRenames(t *testing.T) {
	t.Parallel()

	testCases := slices.Concat(renameCases(), tieCases(), edgeCases())

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			snap := snapshotOf(testCase.revisions, testCase.skip...)

			got, err := NewScanner(&fakeLog{revisions: testCase.revisions}, followConfig()).
				Scan(t.Context(), snap)
			if err != nil {
				t.Fatalf("Scan() = %v", err)
			}

			if !reflect.DeepEqual(got.Births, testCase.want) {
				t.Fatalf("births differ:\n got %v\nwant %v", got.Births, testCase.want)
			}

			off, err := NewScanner(&fakeLog{revisions: testCase.revisions}, config()).
				Scan(t.Context(), snap)
			if err != nil {
				t.Fatalf("Scan() off = %v", err)
			}

			for declKey, birth := range got.Births {
				base, known := off.Births[declKey]
				if !known || birth.At > base.At {
					t.Errorf("%v moved later: %v vs %v", declKey, birth, base)
				}

				if (birth.At < base.At) != (len(birth.Via) > 0) {
					t.Errorf("%v: Via %v does not match the move from %v to %v",
						declKey, birth.Via, base.At, birth.At)
				}
			}
		})
	}
}

func TestScanFollowReadFailures(t *testing.T) {
	t.Parallel()

	revisions := revs(
		rev("r0", add("a.go", "Old "+tailWords)),
		rev("r1", renamed("a.go", "Old "+tailWords, "b.go", "New "+tailWords, 80)),
	)

	testCases := []struct {
		name      string
		failWith  error
		wantErr   error
		wantBorn  entities.Position
		wantFound bool
	}{
		{"the text vanished", ErrBlobMissing, nil, 1, true},
		{"the text read fails", errBoom, errBoom, 0, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			log := &fakeLog{
				revisions: revisions, failWith: testCase.failWith,
				failAfter: map[string]int{"Old " + tailWords: 1},
			}

			got, err := NewScanner(log, followConfig()).Scan(t.Context(), snapshotOf(revisions))
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Scan() = %v, want %v", err, testCase.wantErr)
			}

			if testCase.wantFound && got.Births[key("b.go", "New")].At != testCase.wantBorn {
				t.Fatalf("born at %v, want %v", got.Births[key("b.go", "New")], testCase.wantBorn)
			}
		})
	}
}
