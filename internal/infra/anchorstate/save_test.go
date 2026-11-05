package anchorstate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
)

func TestSave(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		revs      []string
		prior     bool
		wantFiles map[string]string
	}{
		{
			name: "a stop writes its conflicts, the marker that lists them and then the state",
			revs: []string{"aaa111"},
			wantFiles: map[string]string{
				"conflicts/aaa111/00-Run.preimage": "func Run() {}",
				"conflicts/aaa111/00-Run.final":    "func Run() { Z() }",
				"conflicts/" + markerFile:          "aaa111/00-Run.final\naaa111/00-Run.preimage\n",
			},
		},
		{
			name:  "a second stop replaces the first conflicts and the list",
			revs:  []string{"bbb222"},
			prior: true,
			wantFiles: map[string]string{
				"conflicts/bbb222/00-Run.final": "func Run() { Z() }",
				"conflicts/" + markerFile:       "bbb222/00-Run.final\nbbb222/00-Run.preimage\n",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := filepath.Join(t.TempDir(), "state")

			if testCase.prior {
				if err := Save(dir, stateOf("", "aaa111")); err != nil {
					t.Fatalf("prior Save() = %v", err)
				}
			}

			if err := Save(dir, stateOf("", testCase.revs...)); err != nil {
				t.Fatalf("Save() = %v", err)
			}

			for name, want := range testCase.wantFiles {
				got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
				if err != nil || string(got) != want {
					t.Fatalf("%s = %q, %v, want %q", name, got, err, want)
				}
			}

			if _, err := os.Stat(
				filepath.Join(dir, "conflicts", "aaa111"),
			); testCase.prior && err == nil {
				t.Fatal("the first stop's conflicts survived the second")
			}

			assertNoLeftovers(t, dir)
		})
	}
}

func TestSaveKeepsWhatIsNotItsOwn(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		files map[string]string
	}{
		{name: "a file of the user", files: map[string]string{"notes.txt": "mine"}},
		{
			name:  "the layers and the cache of a run",
			files: map[string]string{".redigit-layers.json": "{}", ".redigit-cache/blob": "b"},
		},
		{
			name: "names that only start like redigit's",
			files: map[string]string{
				".redigit-old-mine/x": "o", ".redigit-stage-mine": "s", ".redigit-tmp-mine": "t",
				".redigit-continue.json.bak": "c",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := filepath.Join(t.TempDir(), "state")
			if err := Save(dir, stateOf("", "aaa111")); err != nil {
				t.Fatalf("first Save() = %v", err)
			}

			writeFiles(t, dir, testCase.files)

			if err := Save(dir, stateOf("", "bbb222")); err != nil {
				t.Fatalf("Save() = %v", err)
			}

			for name, want := range testCase.files {
				got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
				if err != nil || string(got) != want {
					t.Fatalf("%s = %q, %v, want it kept as %q", name, got, err, want)
				}
			}
		})
	}
}

func TestSaveRecordsWhoMadeTheDirectory(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		existing    bool
		resaves     bool
		wantCreated bool
	}{
		{name: "a directory Save made", wantCreated: true},
		{name: "a directory that was already there", existing: true},
		{name: "a second Save keeps what the first recorded", resaves: true, wantCreated: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := filepath.Join(t.TempDir(), "state")

			if testCase.existing {
				if err := os.Mkdir(dir, dirMode); err != nil {
					t.Fatal(err)
				}
			}

			saves := []string{"aaa111"}
			if testCase.resaves {
				saves = append(saves, "bbb222")
			}

			for _, rev := range saves {
				if err := Save(dir, stateOf("", rev)); err != nil {
					t.Fatalf("Save() = %v", err)
				}
			}

			state, err := Load(dir)
			if err != nil || state.Created != testCase.wantCreated {
				t.Fatalf("Load() = %+v, %v, want created %v", state, err, testCase.wantCreated)
			}
		})
	}
}

type refusalCase struct {
	name     string
	saved    bool
	files    map[string]string
	dirs     []string
	links    map[string]string
	revs     []string
	wantErr  error
	wantName string
}

func TestSaveRefusals(t *testing.T) {
	t.Parallel()

	checkSaveRefuses(t, []refusalCase{
		{
			name: "a rev that climbs out of the directory", revs: []string{"../escape"},
			wantErr: ErrUnsafe,
		},
		{name: "a rev with a separator", revs: []string{"a/b"}, wantErr: ErrUnsafe},
		{name: "an empty rev", revs: []string{""}, wantErr: ErrUnsafe},
		{name: "an absolute rev", revs: []string{"/abs"}, wantErr: ErrUnsafe},
		{name: "a rev with a line break", revs: []string{"a\nb"}, wantErr: ErrUnsafe},
		{
			name:     "a state.json of someone else",
			files:    map[string]string{StateFile: `{"mine":true}`},
			wantErr:  ErrForeign,
			wantName: StateFile,
		},
		{
			name: "a state.json with no marker", files: map[string]string{StateFile: `{}`},
			wantErr: ErrForeign, wantName: StateFile,
		},
		{
			name: "a state.json that is null", files: map[string]string{StateFile: `null`},
			wantErr: ErrForeign, wantName: StateFile,
		},
		{
			name:  "a conflicts directory with no marker",
			files: map[string]string{"conflicts/data.txt": "tracked"}, wantErr: ErrForeign,
			wantName: "conflicts",
		},
		{
			name: "a conflicts file", files: map[string]string{"conflicts": "mine"},
			wantErr: ErrForeign, wantName: "conflicts",
		},
	})
}

func TestSaveRefusesWhatLiesInConflicts(t *testing.T) {
	t.Parallel()

	checkSaveRefuses(t, []refusalCase{
		{
			name: "an empty conflicts directory the user made", dirs: []string{"conflicts"},
			wantErr: ErrForeign, wantName: "conflicts",
		},
		{
			name: "a conflicts that is a link", dirs: []string{"target"},
			links:   map[string]string{"conflicts": "target"},
			wantErr: ErrForeign, wantName: "conflicts",
		},
		{
			name: "a file of the user beside the texts", saved: true,
			files:   map[string]string{"conflicts/notes.txt": "mine"},
			wantErr: ErrForeign, wantName: "notes.txt",
		},
		{
			name: "a file of the user inside a rev directory", saved: true,
			files:   map[string]string{"conflicts/aaa111/my-edit.go": "mine"},
			wantErr: ErrForeign, wantName: "my-edit.go",
		},
		{
			name: "a directory of the user inside conflicts", saved: true,
			files:   map[string]string{"conflicts/mine/x": "mine"},
			wantErr: ErrForeign, wantName: "mine",
		},
	})
}

func checkSaveRefuses(t *testing.T, testCases []refusalCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := filepath.Join(t.TempDir(), "state")

			if testCase.saved {
				if err := Save(dir, stateOf("", "aaa111")); err != nil {
					t.Fatalf("first Save() = %v", err)
				}
			}

			writeFiles(t, dir, testCase.files)
			writeLayout(t, dir, testCase.dirs, testCase.links)

			before := snapshot(t, dir)

			revs := append([]string{"bbb222"}, testCase.revs...)
			if testCase.revs != nil {
				revs = testCase.revs
			}

			err := Save(dir, stateOf("", revs...))
			if !errors.Is(err, testCase.wantErr) ||
				!strings.Contains(err.Error(), testCase.wantName) {
				t.Fatalf("Save() = %v, want %v naming %q", err, testCase.wantErr, testCase.wantName)
			}

			if after := snapshot(t, dir); after != before {
				t.Fatalf("a refused Save touched the directory:\n%s\nwas\n%s", after, before)
			}
		})
	}
}

func TestSaveRefusesAFileAsDirectory(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(file, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := Save(file, stateOf("", "aaa111"))
	if !errors.Is(err, ErrForeign) || !strings.Contains(err.Error(), file) {
		t.Fatalf("Save() = %v, want ErrForeign naming %s", err, file)
	}

	if got, readErr := os.ReadFile(file); readErr != nil || string(got) != "mine" {
		t.Fatalf("the file is %q, %v", got, readErr)
	}
}

func TestSaveKeepsWhatItWritesPrivate(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "state")
	if err := Save(dir, stateOf(anchoring.PolicyFold, "aaa111")); err != nil {
		t.Fatalf("Save() = %v", err)
	}

	if err := WriteConfig(dir, []byte("{}")); err != nil {
		t.Fatalf("WriteConfig() = %v", err)
	}

	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		info, infoErr := entry.Info()
		if infoErr != nil {
			return fmt.Errorf("stat an entry: %w", infoErr)
		}

		if forbidden := os.FileMode(0o177); !entry.IsDir() && info.Mode().Perm()&forbidden != 0 {
			t.Errorf("%s is %v, want owner read and write only", path, info.Mode().Perm())
		}

		if forbidden := os.FileMode(0o027); entry.IsDir() && info.Mode().Perm()&forbidden != 0 {
			t.Errorf(
				"%s is %v, want no group write and nothing for others",
				path,
				info.Mode().Perm(),
			)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSaveFailures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		setup func(t *testing.T, base string) string
	}{
		{
			name: "a parent that is a regular file",
			setup: func(t *testing.T, base string) string {
				t.Helper()

				writeFiles(t, base, map[string]string{"parent": "mine"})

				return filepath.Join(base, "parent", "state")
			},
		},
		{
			name: "a leftover stage that cannot be cleared",
			setup: func(t *testing.T, base string) string {
				t.Helper()

				dir := filepath.Join(base, "state")
				writeFiles(t, dir, map[string]string{
					stageName + "/" + markerFile: "x/y\n", stageName + "/x/y": "z",
				})
				lock(t, filepath.Join(dir, stageName))

				return dir
			},
		},
		{
			name: "a state directory that cannot be written",
			setup: func(t *testing.T, base string) string {
				t.Helper()

				dir := filepath.Join(base, "state")
				writeFiles(t, dir, nil)
				lock(t, dir)

				return dir
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if os.Geteuid() == 0 {
				t.Skip("root ignores directory permissions")
			}

			dir := testCase.setup(t, t.TempDir())

			if err := Save(dir, stateOf("", "aaa111")); err == nil {
				t.Fatal("Save() = nil, want a refusal")
			}
		})
	}
}

func TestWriteConfig(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		dir     func(t *testing.T) string
		wantErr bool
	}{
		{name: "the derived config lands in the state directory", dir: func(t *testing.T) string {
			t.Helper()

			return t.TempDir()
		}},
		{name: "a directory that is not there is refused", dir: func(t *testing.T) string {
			t.Helper()

			return filepath.Join(t.TempDir(), "missing")
		}, wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := testCase.dir(t)

			err := WriteConfig(dir, []byte(`{"a":1}`))
			if (err != nil) != testCase.wantErr {
				t.Fatalf("WriteConfig() = %v, wantErr %v", err, testCase.wantErr)
			}

			if testCase.wantErr {
				return
			}

			got, readErr := os.ReadFile(filepath.Join(dir, DerivedFile))
			if readErr != nil || string(got) != `{"a":1}` {
				t.Fatalf("config = %q, %v", got, readErr)
			}

			assertNoLeftovers(t, dir)
		})
	}
}
