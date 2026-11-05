package anchorstate

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
)

func TestLoad(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	want := stateOf(anchoring.PolicyWIP, "aaa111")

	if err := Save(dir, want); err != nil {
		t.Fatalf("Save() = %v", err)
	}

	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}

	if !slices.Equal(got.Args, want.Args) || got.Commit != "c1" || got.Digest != "d1" ||
		got.Decisions["decided"] != anchoring.PolicyWIP || got.Marker != marker ||
		len(got.Conflicts) != 1 || got.Conflicts[0].Rev != "aaa111" ||
		got.Conflicts[0].Decls[0].Key != want.Conflicts[0].Decls[0].Key {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadRefusals(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		file    string
		wantErr error
	}{
		{name: "no state is ErrNoState", wantErr: ErrNoState},
		{name: "a state that is not json is foreign", file: "{", wantErr: ErrForeign},
		{name: "an empty object has no marker", file: `{}`, wantErr: ErrForeign},
		{name: "null has no marker", file: `null`, wantErr: ErrForeign},
		{name: "an object of someone else", file: `{"mine":true}`, wantErr: ErrForeign},
		{name: "an args object with no marker", file: `{"args":[]}`, wantErr: ErrForeign},
		{
			name:    "a marked state with no command line is corrupt",
			file:    `{"marker":"` + marker + `","args":[]}`,
			wantErr: ErrCorrupt,
		},
		{name: "a state.json that is a directory", file: "<dir>", wantErr: ErrCorrupt},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()

			switch testCase.file {
			case "":
			case "<dir>":
				if err := os.Mkdir(filepath.Join(dir, StateFile), dirMode); err != nil {
					t.Fatal(err)
				}
			default:
				writeFiles(t, dir, map[string]string{StateFile: testCase.file})
			}

			_, err := Load(dir)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Load() = %v, want %v", err, testCase.wantErr)
			}

			if errors.Is(err, ErrForeign) && !strings.Contains(err.Error(), StateFile) {
				t.Fatalf("Load() = %v, want the file named", err)
			}
		})
	}
}

func TestLoadChecksTheConflictsDirectory(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		isSaved bool
		files   map[string]string
		dirs    []string
		links   map[string]string
		wantErr error
	}{
		{
			name:    "a conflicts directory with no marker",
			files:   map[string]string{StateFile: oursState, "conflicts/data.txt": "mine"},
			wantErr: ErrForeign,
		},
		{
			name: "a conflicts that is a link", files: map[string]string{StateFile: oursState},
			dirs: []string{"target"}, links: map[string]string{"conflicts": "target"},
			wantErr: ErrForeign,
		},
		{
			name:  "an empty conflicts directory beside a state of ours",
			files: map[string]string{StateFile: oursState}, dirs: []string{"conflicts"},
		},
		{
			name: "a marked conflicts directory with a file of the user", isSaved: true,
			files: map[string]string{"conflicts/notes.txt": "mine"},
		},
		{
			name:    "a conflicts directory with no marker and no state",
			files:   map[string]string{"conflicts/data.txt": "mine"},
			wantErr: ErrNoState,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := filepath.Join(t.TempDir(), "state")

			if testCase.isSaved {
				if err := Save(dir, stateOf("", "aaa111")); err != nil {
					t.Fatalf("Save() = %v", err)
				}
			}

			writeFiles(t, dir, testCase.files)
			writeLayout(t, dir, testCase.dirs, testCase.links)

			_, err := Load(dir)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Load() = %v, want %v", err, testCase.wantErr)
			}

			if errors.Is(err, ErrForeign) && !strings.Contains(err.Error(), conflictsDir) {
				t.Fatalf("Load() = %v, want conflicts named", err)
			}
		})
	}
}

func TestFresh(t *testing.T) {
	t.Parallel()

	state := State{Commit: "c1", Digest: "d1"}

	testCases := []struct {
		name    string
		commit  string
		digest  string
		wantErr error
	}{
		{name: "the same commit and config", commit: "c1", digest: "d1"},
		{name: "a moved revision", commit: "c2", digest: "d1", wantErr: ErrStale},
		{name: "an edited config", commit: "c1", digest: "d2", wantErr: ErrStale},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := state.Fresh(testCase.commit, testCase.digest)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Fresh() = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestDigestFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	first := filepath.Join(dir, "a.json")
	same := filepath.Join(dir, "b.json")
	other := filepath.Join(dir, "c.json")

	for path, body := range map[string]string{first: "{}", same: "{}", other: "{ }"} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	left, errLeft := DigestFile(first)
	right, errRight := DigestFile(same)
	diff, errDiff := DigestFile(other)
	_, errMissing := DigestFile(filepath.Join(dir, "none.json"))

	if errLeft != nil || errRight != nil || errDiff != nil || left != right || left == diff {
		t.Fatalf("digests = %q %q %q, errs %v %v %v", left, right, diff, errLeft, errRight, errDiff)
	}

	if errMissing == nil {
		t.Fatal("DigestFile() of a missing file = nil, want an error")
	}
}
