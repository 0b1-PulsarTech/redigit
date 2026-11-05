package anchorstate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
)

func TestStemOf(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("L", 300)
	accents := strings.Repeat("é", 100)

	testCases := []struct {
		name       string
		index      int
		decl       string
		want       string
		isTruncate bool
	}{
		{name: "a short name", index: 3, decl: "Run", want: "03-Run"},
		{name: "a slash, a backslash and a space", decl: "a/b\\c d", want: "00-a_b_c_d"},
		{name: "a method name", index: 12, decl: "(*T).Run", want: "12-(*T).Run"},
		{
			name: "a name at the limit",
			decl: strings.Repeat("n", maxName),
			want: "00-" + strings.Repeat("n", maxName),
		},
		{name: "a name past the limit", decl: long, isTruncate: true},
		{name: "a long name of two byte characters", decl: accents, isTruncate: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := stemOf(testCase.index, testCase.decl)
			if !testCase.isTruncate {
				if got != testCase.want {
					t.Fatalf("stemOf() = %q, want %q", got, testCase.want)
				}

				return
			}

			if len(got) > maxName+len("00-") || !utf8.ValidString(got) ||
				got != stemOf(testCase.index, testCase.decl) {
				t.Fatalf(
					"stemOf() = %q, want a stable valid stem of at most %d bytes",
					got,
					maxName,
				)
			}
		})
	}
}

func TestFilesRefuseWhatTheyCannotUse(t *testing.T) {
	t.Parallel()

	nul := conflictFor("aaa111")
	nul.Decls[0].Key.Name = "a\x00b"

	testCases := []struct {
		name string
		run  func(file string) error
	}{
		{name: "a conflict directory that is a file", run: func(file string) error {
			return writeConflict(file, conflictFor("aaa111"))
		}},
		{name: "a declaration name the filesystem refuses", run: func(file string) error {
			return writeConflict(filepath.Dir(file), nul)
		}},
		{name: "a listing of a file", run: func(file string) error {
			_, err := reservedIn(file)

			return err
		}},
		{name: "a marker beside a file", run: func(file string) error {
			_, _, err := readMarker(file)

			return err
		}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			file := filepath.Join(t.TempDir(), "plain")
			if err := os.WriteFile(file, []byte("x"), fileMode); err != nil {
				t.Fatal(err)
			}

			if err := testCase.run(file); err == nil {
				t.Fatal("got nil, want the refusal")
			}
		})
	}
}

func TestStemOfTellsLongNamesApart(t *testing.T) {
	t.Parallel()

	head := strings.Repeat("L", 300)

	if stemOf(0, head+"A") == stemOf(0, head+"B") || stemOf(0, head) == stemOf(1, head) {
		t.Fatal("two long names, or one name at two places, share a stem")
	}
}

func TestWriteConflictKeepsLongNames(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("L", 300)
	other := strings.Repeat("L", 299) + "M"
	state := stateOf("", "aaa111")
	state.Conflicts[0].Decls = append(state.Conflicts[0].Decls,
		anchoring.ConflictDecl{
			Key:      entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: long},
			Preimage: "one",
		},
		anchoring.ConflictDecl{
			Key:      entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: other},
			Preimage: "two",
		},
	)

	dir := filepath.Join(t.TempDir(), "state")
	if err := Save(dir, state); err != nil {
		t.Fatalf("Save() = %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(dir, "conflicts", "aaa111"))
	if err != nil || len(entries) != 6 {
		t.Fatalf("conflict directory holds %v, %v, want 6 files", entries, err)
	}

	for _, entry := range entries {
		if len(entry.Name()) > 200 {
			t.Fatalf("file name %q is %d bytes", entry.Name(), len(entry.Name()))
		}
	}
}
