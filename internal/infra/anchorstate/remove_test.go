package anchorstate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const oursState = `{"marker":"` + marker + `","args":["emit"]}`

type removeCase struct {
	name       string
	isSaved    bool
	isExisting bool
	isDerived  bool
	files      map[string]string
	dirs       []string
	links      map[string]string
	gone       []string
	wantErr    error
	wantName   string
	wantKept   []string
	wantDir    bool
}

func prepareRemove(t *testing.T, testCase removeCase) string {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "state")

	if testCase.isExisting {
		if err := os.Mkdir(dir, dirMode); err != nil {
			t.Fatal(err)
		}
	}

	if testCase.isSaved {
		if err := Save(dir, stateOf("", "aaa111", "bbb222")); err != nil {
			t.Fatalf("Save() = %v", err)
		}
	}

	if testCase.isDerived {
		if err := WriteConfig(dir, []byte("{}")); err != nil {
			t.Fatal(err)
		}
	}

	for _, name := range testCase.gone {
		if err := os.Remove(filepath.Join(dir, filepath.FromSlash(name))); err != nil {
			t.Fatal(err)
		}
	}

	if testCase.files != nil {
		writeFiles(t, dir, testCase.files)
	}

	if testCase.dirs != nil || testCase.links != nil {
		writeLayout(t, dir, testCase.dirs, testCase.links)
	}

	return dir
}

func TestRemove(t *testing.T) {
	t.Parallel()

	stage := map[string]string{
		stageName + "/" + markerFile: "conflicts/x\n", stageName + "/conflicts/x": "x",
		tmpPrefix + "1": "t", oldName + "/" + markerFile: "o/y\n", oldName + "/o/y": "o",
	}
	bystanders := map[string]string{
		".redigit-layers.json": "{}", ".redigit-cache/blob": "b", ".redigit-old-mine/z": "z",
		".redigit-stage-mine": "s", ".redigit-tmp-mine": "t", ".redigit-continue.json.bak": "c",
	}
	kept := []string{
		".redigit-layers.json", ".redigit-cache/blob", ".redigit-old-mine/z",
		".redigit-stage-mine", ".redigit-tmp-mine", ".redigit-continue.json.bak",
	}

	checkRemove(t, []removeCase{
		{
			name:    "an abort removes what Save and continue made and the directory Save made",
			isSaved: true, isDerived: true, files: stage,
		},
		{
			name: "an abort leaves a directory that was already there", isSaved: true,
			isExisting: true, isDerived: true, wantDir: true,
		},
		{
			name:    "an abort leaves a bystander file and the config of the user",
			isSaved: true, isDerived: true,
			files:    map[string]string{"notes.txt": "mine", "config.json": "{}"},
			wantKept: []string{"notes.txt", "config.json"}, wantDir: true,
		},
		{
			name: "an abort leaves a file inside the conflicts directory", isSaved: true,
			files:    map[string]string{"conflicts/notes.txt": "mine"},
			wantKept: []string{"conflicts/notes.txt"}, wantDir: true,
		},
		{
			name: "an abort leaves a file of the user inside a rev directory", isSaved: true,
			files:    map[string]string{"conflicts/aaa111/my-edit.go": "mine"},
			wantKept: []string{"conflicts/aaa111/my-edit.go"}, wantDir: true,
		},
		{
			name: "an abort leaves a directory of the user inside conflicts", isSaved: true,
			files:    map[string]string{"conflicts/mine/x": "mine"},
			wantKept: []string{"conflicts/mine/x"}, wantDir: true,
		},
		{
			name: "an abort leaves the names that only start like redigit's", isSaved: true,
			isDerived: true, files: bystanders, wantKept: kept, wantDir: true,
		},
		{
			name:  "an abort with only the leftovers of a save",
			files: stage, wantDir: true,
		},
	})
}

func TestRemoveGoesOnPastWhatItCannotName(t *testing.T) {
	t.Parallel()

	checkRemove(t, []removeCase{
		{
			name: "an abort of a text that is already gone", isSaved: true,
			gone: []string{"conflicts/aaa111/00-Run.final"},
		},
		{
			name: "an abort does not follow a rev link out of conflicts", isSaved: true,
			gone: []string{
				"conflicts/aaa111/00-Run.final", "conflicts/aaa111/00-Run.preimage",
				"conflicts/aaa111",
			},
			files:    map[string]string{"outside/00-Run.final": "mine"},
			links:    map[string]string{"conflicts/aaa111": "../outside"},
			wantKept: []string{"outside/00-Run.final", "conflicts/aaa111"}, wantDir: true,
		},
		{
			name: "an abort does not follow a marker that lists paths outside",
			files: map[string]string{
				StateFile:                 oursState,
				"notes.txt":               "mine",
				"conflicts/" + markerFile: "../notes.txt\n../../x\naaa111/../../notes.txt\n/etc/passwd\n",
			},
			wantKept: []string{"notes.txt"}, wantDir: true,
		},
		{
			name:    "an abort removes a marked state that lost its command line",
			files:   map[string]string{StateFile: `{"marker":"` + marker + `","args":[]}`},
			wantDir: true,
		},
		{
			name:  "an abort removes the empty conflicts directory an interrupted abort left",
			files: map[string]string{StateFile: oursState}, dirs: []string{"conflicts"},
			wantDir: true,
		},
		{
			name:     "an abort leaves a conflicts directory with no marker beside a state",
			files:    map[string]string{StateFile: oursState, "conflicts/data.txt": "mine"},
			wantKept: []string{"conflicts/data.txt"}, wantDir: true,
		},
		{
			name:     "an abort leaves a conflicts link beside a state",
			files:    map[string]string{StateFile: oursState},
			dirs:     []string{"target"},
			links:    map[string]string{"conflicts": "target"},
			wantKept: []string{"conflicts", "target"}, wantDir: true,
		},
	})
}

func TestRemoveFailuresCanBeRetried(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		path string
		mode os.FileMode
	}{
		{name: "a directory that cannot be listed", mode: 0o300},
		{name: "a directory that cannot be written", mode: 0o500},
		{name: "a conflicts directory that cannot be listed", path: conflictsDir, mode: 0o300},
		{name: "a conflicts directory that cannot be written", path: conflictsDir, mode: 0o500},
		{name: "a rev directory that cannot be emptied", path: "conflicts/aaa111", mode: 0o500},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if os.Geteuid() == 0 {
				t.Skip("root ignores directory permissions")
			}

			dir := filepath.Join(t.TempDir(), "state")
			if err := Save(dir, stateOf("", "aaa111")); err != nil {
				t.Fatalf("Save() = %v", err)
			}

			locked := filepath.Join(dir, filepath.FromSlash(testCase.path))

			t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })

			if err := os.Chmod(locked, testCase.mode); err != nil {
				t.Fatal(err)
			}

			if err := Remove(dir); err == nil {
				t.Fatal("Remove() = nil, want the failure")
			}

			if err := os.Chmod(locked, 0o700); err != nil {
				t.Fatal(err)
			}

			if _, err := Load(dir); err != nil {
				t.Fatalf("Load() after the failure = %v, want the state kept for the retry", err)
			}

			if err := Remove(dir); err != nil {
				t.Fatalf("Remove() after the failure = %v, want the way forward", err)
			}

			if _, err := os.Stat(dir); err == nil {
				t.Fatal("the retry left the directory")
			}
		})
	}
}

func TestRemoveKeepsTheStateWhileAFailureStandsBesideAForeignFile(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		files map[string]string
	}{
		{
			name:  "a foreign file in the old conflicts",
			files: map[string]string{oldName + "/" + markerFile: "", oldName + "/mine": "m"},
		},
		{name: "a foreign temporary dir", files: map[string]string{tmpPrefix + "5/mine": "m"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if os.Geteuid() == 0 {
				t.Skip("root ignores directory permissions")
			}

			dir := filepath.Join(t.TempDir(), "state")
			if err := Save(dir, stateOf("", "aaa111")); err != nil {
				t.Fatalf("Save() = %v", err)
			}

			writeFiles(t, dir, testCase.files)
			writeFiles(t, dir, map[string]string{
				stageName + "/" + markerFile: "z/t\n", stageName + "/z/t": "t",
			})

			locked := filepath.Join(dir, stageName, "z")
			lock(t, locked)

			err := Remove(dir)
			if err == nil || errors.Is(err, ErrForeign) {
				t.Fatalf("Remove() = %v, want the failure and not the foreign file", err)
			}

			if _, loadErr := Load(dir); loadErr != nil {
				t.Fatalf("Load() after the failure = %v, want the state kept", loadErr)
			}

			if err = os.Chmod(locked, 0o700); err != nil {
				t.Fatal(err)
			}

			if err = Remove(dir); err != nil {
				t.Fatalf("Remove() after the failure = %v, want the way forward", err)
			}
		})
	}
}

func TestRemoveKill(t *testing.T) {
	t.Parallel()

	for _, step := range []string{stepListed, stepMarker, stepConflicts, stepNamed} {
		t.Run(step, func(t *testing.T) {
			t.Parallel()

			dir := filepath.Join(t.TempDir(), "state")
			if err := Save(dir, stateOf("", "aaa111", "bbb222")); err != nil {
				t.Fatalf("Save() = %v", err)
			}

			if err := WriteConfig(dir, []byte("{}")); err != nil {
				t.Fatal(err)
			}

			writeFiles(t, dir, map[string]string{
				oldName + "/" + markerFile: "o/y\n", oldName + "/o/y": "o", tmpPrefix + "7": "t",
			})

			if err := (saver{hook: killAt(step)}).remove(dir); !errors.Is(err, errKilled) {
				t.Fatalf("remove() = %v, want the kill", err)
			}

			if _, err := Load(dir); err != nil {
				t.Fatalf("Load() after the kill = %v, want the state kept for the retry", err)
			}

			if err := Remove(dir); err != nil {
				t.Fatalf("Remove() after the kill = %v, want the way forward", err)
			}

			if _, err := os.Stat(dir); err == nil {
				t.Fatalf("the retry left the directory: %v", snapshot(t, dir))
			}
		})
	}
}

func TestRemoveRefusals(t *testing.T) {
	t.Parallel()

	foreign := map[string]string{
		StateFile: `{"mine":true}`, "config.json": "{}", "conflicts/keep/y": "y", "notes.txt": "n",
	}

	checkRemove(t, []removeCase{
		{
			name: "a state of someone else is refused and nothing is removed", files: foreign,
			wantErr: ErrForeign, wantName: StateFile, wantKept: []string{StateFile},
			wantDir: true,
		},
		{
			name:     "a conflicts directory with no marker is not a state",
			files:    map[string]string{"conflicts/data.txt": "tracked", "notes.txt": "n"},
			wantErr:  ErrNoState,
			wantKept: []string{"conflicts/data.txt"}, wantDir: true,
		},
		{
			name: "an empty conflicts directory of the user is not a state",
			dirs: []string{"conflicts"}, wantErr: ErrNoState, wantKept: []string{"conflicts"},
			wantDir: true,
		},
		{
			name: "a conflicts link is not a state", dirs: []string{"target"},
			links:   map[string]string{"conflicts": "target"},
			wantErr: ErrNoState, wantKept: []string{"conflicts", "target"}, wantDir: true,
		},
		{
			name:     "a state.json that is not json is refused",
			files:    map[string]string{StateFile: "{"},
			wantErr:  ErrForeign,
			wantName: StateFile,
			wantKept: []string{StateFile},
			wantDir:  true,
		},
		{
			name:    "a state.json that cannot be read is refused",
			files:   map[string]string{StateFile + "/x": "x"},
			wantErr: ErrCorrupt, wantName: StateFile, wantKept: []string{StateFile}, wantDir: true,
		},
		{name: "nothing to abort is ErrNoState", wantErr: ErrNoState},
		{
			name:       "an empty directory is ErrNoState",
			isExisting: true,
			wantErr:    ErrNoState,
			wantDir:    true,
		},
		{
			name:    "a directory with only a bystander is ErrNoState",
			files:   map[string]string{"notes.txt": "mine"},
			wantErr: ErrNoState, wantKept: []string{"notes.txt"}, wantDir: true,
		},
		{
			name: "a directory with only names that start like redigit's is ErrNoState",
			files: map[string]string{
				".redigit-layers.json": "{}", ".redigit-cache/blob": "b", ".redigit-tmp-mine": "t",
			},
			wantErr:  ErrNoState,
			wantKept: []string{".redigit-layers.json", ".redigit-cache/blob", ".redigit-tmp-mine"},
			wantDir:  true,
		},
	})
}

func checkRemove(t *testing.T, testCases []removeCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := prepareRemove(t, testCase)
			sibling := filepath.Join(filepath.Dir(dir), "other")

			if err := os.MkdirAll(sibling, dirMode); err != nil {
				t.Fatal(err)
			}

			before := snapshot(t, dir)

			err := Remove(dir)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Remove() = %v, want %v", err, testCase.wantErr)
			}

			if testCase.wantName != "" && !strings.Contains(err.Error(), testCase.wantName) {
				t.Fatalf("Remove() = %v, want %q named", err, testCase.wantName)
			}

			if testCase.wantErr != nil && snapshot(t, dir) != before {
				t.Fatalf(
					"a refused Remove touched the directory:\n%s\nwas\n%s",
					snapshot(t, dir),
					before,
				)
			}

			assertAbortResult(t, dir, sibling, testCase)
		})
	}
}

func assertAbortResult(t *testing.T, dir, sibling string, testCase removeCase) {
	t.Helper()

	if _, err := os.Stat(sibling); err != nil {
		t.Fatalf("a sibling directory was touched: %v", err)
	}

	if _, err := os.Stat(dir); (err == nil) != testCase.wantDir {
		t.Fatalf("directory kept = %v, want %v", err == nil, testCase.wantDir)
	}

	for _, name := range testCase.wantKept {
		if _, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(name))); err != nil {
			t.Fatalf("%s did not survive: %v", name, err)
		}
	}

	if testCase.wantErr != nil {
		return
	}

	for _, name := range []string{StateFile, DerivedFile, "conflicts/bbb222/00-Run.final"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(name))); err == nil {
			t.Fatalf("%s survived the abort", name)
		}
	}

	assertNoLeftovers(t, dir)
}
