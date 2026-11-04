package write

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/rewriting"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/subjecting"
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
			[]rewriting.Row{
				{Position: 1, Subject: "s", Blobs: blobs, ID: subjecting.ID(nil, []string{"a.go"})},
				{Position: 0, Subject: "t", ID: subjecting.ID(nil, nil)},
			},
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

func TestReadRowsKeepsWIPAndID(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		content string
		wantWIP bool
		wantID  string
	}{
		{
			name: "a wip row hashes its chunk keys and blob paths",
			content: `{"position":0,"subject":"s","is_wip":true,` +
				`"chunks":[{"key":"k2","span":[0,1],"unit":"u"},{"key":"k1","span":[9,12],"unit":"u"}],` +
				`"blobs":[{"path":"a.go","mode":"100644","handle":"h"}]}`,
			wantWIP: true,
			wantID:  subjecting.ID([]string{"k1", "k2"}, []string{"a.go"}),
		},
		{
			name:    "a row without chunks hashes its blobs alone",
			content: `{"position":0,"blobs":[{"path":"go.mod","mode":"100644","handle":"h"}]}`,
			wantID:  subjecting.ID(nil, []string{"go.mod"}),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "m.jsonl")
			if err := os.WriteFile(path, []byte(testCase.content), 0o600); err != nil {
				t.Fatalf("write manifest: %v", err)
			}

			got, err := readRows(path)
			if err != nil || len(got) != 1 {
				t.Fatalf("readRows() = %+v, %v", got, err)
			}

			if got[0].IsWIP != testCase.wantWIP || got[0].ID != testCase.wantID {
				t.Fatalf(
					"readRows() row = %+v, want wip %v id %q",
					got[0],
					testCase.wantWIP,
					testCase.wantID,
				)
			}
		})
	}
}
