package write

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/rewriting"
)

func TestReadRows(t *testing.T) {
	t.Parallel()

	blobs := []rewriting.BlobRef{{Path: "a.go", Mode: "100644", Handle: "h"}}

	testCases := []struct {
		name    string
		content string
		want    []rewriting.Row
		wantErr error
	}{
		{
			"lines with extra fields and blank lines",
			`{"position":1,"subject":"s","shape":"x","blobs":[{"path":"a.go","mode":"100644","handle":"h"}]}` +
				"\n\n" + `{"position":0,"subject":"t","blobs":[]}` + "\n",
			[]rewriting.Row{{Position: 1, Subject: "s", Blobs: blobs}, {Position: 0, Subject: "t"}},
			nil,
		},
		{"empty manifest", "", nil, nil},
		{"garbage line", "{nope", nil, rewriting.ErrManifest},
		{"row without a position", `{"subject":"s","blobs":[]}`, nil, rewriting.ErrManifest},
		{"row with a null position", `{"position":null}`, nil, rewriting.ErrManifest},
		{"blob with an empty mode", noModeRow(`"mode":"",`), nil, rewriting.ErrManifest},
		{"blob with no mode", noModeRow(``), nil, rewriting.ErrManifest},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "m.jsonl")
			if err := os.WriteFile(path, []byte(testCase.content), 0o600); err != nil {
				t.Fatalf("write manifest: %v", err)
			}

			got, err := readRows(path)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("readRows() err = %v, want %v", err, testCase.wantErr)
			}

			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("readRows() = %+v, want %+v", got, testCase.want)
			}
		})
	}
}

func TestReadRowsMissingFile(t *testing.T) {
	t.Parallel()

	if _, err := readRows(filepath.Join(t.TempDir(), "absent")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("readRows() err = %v, want %v", err, fs.ErrNotExist)
	}
}

func TestReadSubjects(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		content string
		want    map[string]string
		wantErr error
	}{
		{
			"position to message",
			`{"0":"a","3":"b\n"}`,
			map[string]string{"0": "a", "3": "b\n"},
			nil,
		},
		{"not an object", `[1]`, nil, rewriting.ErrManifest},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "s.json")
			if err := os.WriteFile(path, []byte(testCase.content), 0o600); err != nil {
				t.Fatalf("write subjects: %v", err)
			}

			got, err := readSubjects(path)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("readSubjects() err = %v, want %v", err, testCase.wantErr)
			}

			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("readSubjects() = %v, want %v", got, testCase.want)
			}
		})
	}

	if _, err := readSubjects(
		filepath.Join(t.TempDir(), "absent"),
	); !errors.Is(
		err,
		fs.ErrNotExist,
	) {
		t.Fatalf("readSubjects() err = %v, want %v", err, fs.ErrNotExist)
	}
}

func noModeRow(modeField string) string {
	return `{"position":0,"blobs":[{"path":"a",` + modeField + `"handle":"h"}]}`
}
