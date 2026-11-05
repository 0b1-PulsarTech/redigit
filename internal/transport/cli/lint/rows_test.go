package lint

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/linting"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/rewriting"
	"github.com/wrapped-owls/redigit/internal/transport/cli/manifest"
	"github.com/wrapped-owls/redigit/internal/transport/cli/write"
)

func TestLineCount(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		content string
		want    int
	}{
		{name: "empty file", content: "", want: 0},
		{name: "one line with a newline", content: "a\n", want: 1},
		{name: "one line without a newline", content: "a", want: 1},
		{name: "three lines", content: "a\nb\nc\n", want: 3},
		{name: "a blank line counts", content: "a\n\nb", want: 3},
		{name: "longer than one read buffer", content: strings.Repeat("x\n", 40000), want: 40000},
		{name: "a NUL byte makes it binary and 0 lines", content: "a\nb\x00\nc\n", want: 0},
		{name: "a NUL byte at the end", content: "a\nb\n\x00", want: 0},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := lineCount([]byte(testCase.content)); got != testCase.want {
				t.Fatalf("lineCount() = %d, want %d", got, testCase.want)
			}
		})
	}
}

func TestReadBlobRefusesWhatItCannotRead(t *testing.T) {
	t.Parallel()

	dir := blobDirWith(t, map[string]string{"ok": "a\n"})
	if err := os.Mkdir(filepath.Join(dir, "adir"), 0o700); err != nil {
		t.Fatalf("Mkdir() err = %v", err)
	}

	testCases := []struct {
		name    string
		handle  string
		wantErr bool
		isUse   bool
	}{
		{name: "a plain handle is read", handle: "ok"},
		{name: "a path escape", handle: "../x", wantErr: true, isUse: true},
		{name: "a nested handle", handle: "a/b", wantErr: true, isUse: true},
		{name: "a missing blob", handle: "nope", wantErr: true},
		{name: "a directory", handle: "adir", wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			content, err := readBlob(dir, testCase.handle)
			isUse := errors.Is(err, write.ErrUsage)

			if (err != nil) != testCase.wantErr || isUse != testCase.isUse {
				t.Fatalf("readBlob() = %q, %v", content, err)
			}
		})
	}
}

func TestAddedLinesIsTheNetGrowthOverTheLastRowThatHeldThePath(t *testing.T) {
	t.Parallel()

	five := "a\nb\nc\nd\ne\n"
	plain := func(handle string) []manifest.Blob {
		return []manifest.Blob{{Path: "a.go", Mode: "100644", Handle: handle}}
	}

	testCases := []struct {
		name  string
		blobs map[string]string
		rows  []manifest.Row
		want  measuredRows
	}{
		{
			name:  "first sight adds every line",
			blobs: map[string]string{"v1": "a\nb\n"},
			rows:  []manifest.Row{{Position: 0, Subject: "s", Blobs: plain("v1")}},
			want: measuredRows{measured: true, rows: []linting.Row{{
				Position: 0, Subject: "s",
				Files: []linting.File{{Path: "a.go", Added: 2, Kind: linting.KindSource}},
			}}},
		},
		{
			name:  "a shrink is negative and the next growth is over the shrunk size",
			blobs: map[string]string{"v5": five, "v1": "a\n", "v3": "a\nb\nc\n"},
			rows: []manifest.Row{
				{Position: 0, Subject: "grows", Blobs: plain("v5")},
				{Position: 1, Subject: "shrinks", Blobs: plain("v1")},
				{Position: 2, Subject: "regrows", Blobs: plain("v3")},
			},
			want: measuredRows{measured: true, rows: []linting.Row{
				{Position: 0, Subject: "grows", Files: []linting.File{
					{Path: "a.go", Added: 5, Kind: linting.KindSource},
				}},
				{Position: 1, Subject: "shrinks", Files: []linting.File{
					{Path: "a.go", Added: -4, Kind: linting.KindSource},
				}},
				{Position: 2, Subject: "regrows", Files: []linting.File{
					{Path: "a.go", Added: 2, Kind: linting.KindSource},
				}},
			}},
		},
		{
			name:  "a rewrite at equal length adds nothing",
			blobs: map[string]string{"old": "a\nb\nc\n", "new": "x\ny\nz\n"},
			rows: []manifest.Row{
				{Position: 0, Subject: "first", Blobs: plain("old")},
				{Position: 1, Subject: "rewrite", Blobs: plain("new")},
			},
			want: measuredRows{measured: true, rows: []linting.Row{
				{Position: 0, Subject: "first", Files: []linting.File{
					{Path: "a.go", Added: 3, Kind: linting.KindSource},
				}},
				{Position: 1, Subject: "rewrite", Files: []linting.File{
					{Path: "a.go", Added: 0, Kind: linting.KindSource},
				}},
			}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			testCase.want.decoded = testCase.rows

			got, err := addedLines(testCase.rows, blobDirWith(t, testCase.blobs))
			if err != nil || !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("addedLines() = %+v, %v, want %+v", got, err, testCase.want)
			}
		})
	}
}

func TestAddedLinesCountsNothingForWhatIsNotText(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		blobs map[string]string
		rows  []manifest.Row
		want  measuredRows
	}{
		{
			name:  "a submodule entry is skipped and a binary blob counts 0 lines",
			blobs: map[string]string{"bin": "a\n\x00b\n"},
			rows: []manifest.Row{{Position: 0, Subject: "s", Blobs: []manifest.Blob{
				{Path: "sub", Mode: "160000", Handle: "ignored"},
				{Path: "huge.bin", Mode: "100644", Handle: "bin"},
			}}},
			want: measuredRows{measured: true, rows: []linting.Row{{
				Position: 0, Subject: "s",
				Files: []linting.File{{Path: "huge.bin", Added: 0, Kind: linting.KindOther}},
			}}},
		},
		{
			name: "a row without blobs reads nothing",
			rows: []manifest.Row{{Position: 0, Subject: "empty"}},
			want: measuredRows{rows: []linting.Row{{Position: 0, Subject: "empty"}}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			testCase.want.decoded = testCase.rows

			got, err := addedLines(testCase.rows, blobDirWith(t, testCase.blobs))
			if err != nil || !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("addedLines() = %+v, %v, want %+v", got, err, testCase.want)
			}
		})
	}
}

func TestReadRows(t *testing.T) {
	t.Parallel()

	rowOne := `{"position":0,"subject":"s0","blobs":[{"path":"a.go","mode":"100644","handle":"v1"}]}` + "\n"
	rowTwo := `{"position":1,"subject":"s1","blobs":[{"path":"a.go","mode":"100644","handle":"v2"}]}` + "\n"
	rowThree := `{"position":2,"subject":"s2","blobs":[{"path":"a.go","mode":"100644","handle":"v3"}]}` + "\n"

	testCases := []struct {
		name         string
		manifest     string
		noBlobDir    bool
		wantPosition []int
		wantAdded    []int
		wantErr      error
	}{
		{
			name:         "rows out of position order are read in position order",
			manifest:     rowThree + rowOne + rowTwo,
			wantPosition: []int{0, 1, 2},
			wantAdded:    []int{1, 1, 1},
		},
		{
			name:         "the sort applies without a blob dir too",
			manifest:     rowTwo + rowOne,
			noBlobDir:    true,
			wantPosition: []int{0, 1},
			wantAdded:    []int{0, 0},
		},
		{name: "a manifest without a row is refused", manifest: "", wantErr: rewriting.ErrManifest},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := blobDirWith(t, map[string]string{"v1": "a\n", "v2": "a\nb\n", "v3": "a\nb\nc\n"})
			if testCase.noBlobDir {
				dir = ""
			}

			got, err := readRows(manifestFile(t, testCase.manifest), dir)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("readRows() err = %v, want %v", err, testCase.wantErr)
			}

			positions := make([]int, 0, len(got.rows))
			added := make([]int, 0, len(got.rows))

			for _, readRow := range got.rows {
				positions = append(positions, readRow.Position)

				total := 0
				for _, file := range readRow.Files {
					total += file.Added
				}

				added = append(added, total)
			}

			if testCase.wantErr == nil &&
				(!reflect.DeepEqual(positions, testCase.wantPosition) ||
					!reflect.DeepEqual(added, testCase.wantAdded) ||
					got.measured == testCase.noBlobDir) {
				t.Fatalf("readRows() = %+v, positions %v added %v", got, positions, added)
			}
		})
	}
}
