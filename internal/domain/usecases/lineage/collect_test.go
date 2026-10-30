package lineage

import (
	"maps"
	"reflect"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestScanFollowKeepsTheKeySet(t *testing.T) {
	t.Parallel()

	testCases := make([]offCase, 0, 32)
	testCases = append(testCases, offCases()...)

	for _, follow := range renameCases() {
		testCases = append(testCases, offCase{follow.name, follow.revisions, follow.skip})
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			snap := snapshotOf(testCase.revisions, testCase.skip...)

			off, err := NewScanner(&fakeLog{revisions: testCase.revisions}, config()).
				Scan(t.Context(), snap)
			if err != nil {
				t.Fatalf("Scan(off) = %v", err)
			}

			on, err := NewScanner(&fakeLog{revisions: testCase.revisions}, followConfig()).
				Scan(t.Context(), snap)
			if err != nil {
				t.Fatalf("Scan(on) = %v", err)
			}

			offKeys := slices.SortedFunc(maps.Keys(off.Births), compareKeys)
			onKeys := slices.SortedFunc(maps.Keys(on.Births), compareKeys)

			if !slices.Equal(offKeys, onKeys) {
				t.Errorf("follow changed the keys:\n on %v\noff %v", onKeys, offKeys)
			}

			if !slices.Equal(off.Unreadable, on.Unreadable) {
				t.Errorf("follow changed unreadable: %v vs %v", on.Unreadable, off.Unreadable)
			}
		})
	}
}

func TestParseAllKeepsSpansOnlyForMoves(t *testing.T) {
	t.Parallel()

	const text = "A one two three four"

	revisions := revs(
		rev("r0", add("a.go", text), add("c.go", "C one")),
		rev("r1", renamed("a.go", text, "b.go", text, 100)),
	)

	testCases := []struct {
		name      string
		cfg       Config
		wantSpans []string
	}{
		{"follow off keeps none", config(), nil},
		{"follow on keeps the renamed pair", followConfig(), []string{"a.go", "b.go"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			scanner := NewScanner(&fakeLog{revisions: revisions}, testCase.cfg)

			hist, err := scanner.collect(t.Context(), snapshotOf(revisions))
			if err != nil {
				t.Fatalf("collect() = %v", err)
			}

			units, err := scanner.parseAll(t.Context(), hist.units)
			if err != nil {
				t.Fatalf("parseAll() = %v", err)
			}

			var got []string

			for idx, one := range hist.units {
				if len(units[idx].keys) == 0 {
					t.Fatalf("%s lost its keys", one.path)
				}

				if len(units[idx].cuts) > 0 {
					got = append(got, one.path)
				}
			}

			slices.Sort(got)

			if !slices.Equal(got, testCase.wantSpans) {
				t.Fatalf("spans kept for %v, want %v", got, testCase.wantSpans)
			}
		})
	}
}

func TestScanSortsSightingsByPosition(t *testing.T) {
	t.Parallel()

	const text = "A one two three four"

	birth := entities.Position(0)
	moved := key("b.go", "A")
	step := Rename{At: 1, From: key("a.go", "A"), To: moved, FileScore: 100}

	testCases := []struct {
		name      string
		revisions []Revision
		history   []entities.SourceTouch
		want      Birth
	}{
		{
			"the log is in position order",
			revs(rev("r0", add("a.go", text)), rev("r1", renamed("a.go", text, "b.go", text, 100))),
			[]entities.SourceTouch{{Position: 0, Rev: "r0"}, {Position: 1, Rev: "r1"}},
			Birth{At: birth, Via: []Rename{step}},
		},
		{
			"the log lists the rename before its origin",
			revs(rev("r1", renamed("a.go", text, "b.go", text, 100)), rev("r0", add("a.go", text))),
			[]entities.SourceTouch{{Position: 0, Rev: "r0"}, {Position: 1, Rev: "r1"}},
			Birth{At: birth, Via: []Rename{step}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := NewScanner(&fakeLog{revisions: testCase.revisions}, followConfig()).
				Scan(t.Context(), entities.Snapshot{History: testCase.history})
			if err != nil {
				t.Fatalf("Scan() = %v", err)
			}

			if !reflect.DeepEqual(got.Births[moved], testCase.want) {
				t.Fatalf("birth of %v = %+v, want %+v", moved, got.Births[moved], testCase.want)
			}
		})
	}
}

func TestScanRenameNeedsOneLanguage(t *testing.T) {
	t.Parallel()

	const text = "A one two three four"

	cfg := followConfig()
	cfg.Languages = append(cfg.Languages, Language{
		Extensions: []string{".gox"}, Parser: fakeSyntax{},
	})

	testCases := []struct {
		name     string
		from     string
		fromBlob string
	}{
		{"the old path belongs to another language", "a.gox", text},
		{"the old path belongs to no language", "a.txt", text},
		{"the old blob is unknown", "a.go", ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			moved := Change{
				Path: "b.go", OldPath: testCase.from, Blob: text, OldBlob: testCase.fromBlob,
				Status: StatusRenamed, Similarity: 100,
			}
			revisions := revs(rev("r0", add(testCase.from, text)), rev("r1", moved))

			got, err := NewScanner(&fakeLog{revisions: revisions}, cfg).
				Scan(t.Context(), snapshotOf(revisions))
			if err != nil {
				t.Fatalf("Scan() = %v", err)
			}

			want := Birth{At: 1}
			if birth := got.Births[key("b.go", "A")]; !reflect.DeepEqual(birth, want) {
				t.Fatalf("birth of b.go A = %+v, want %+v", birth, want)
			}
		})
	}
}
