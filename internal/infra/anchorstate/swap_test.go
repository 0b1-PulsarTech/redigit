package anchorstate

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
)

func at(step string, act func()) func(string) error {
	return func(reached string) error {
		if reached == step {
			act()
		}

		return nil
	}
}

func TestSaveFailureKeepsTheOldStateAndCanBeRetried(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		prep func(dir string)
		hook func(dir string) func(string) error
	}{
		{
			name: "the old conflicts cannot be set aside",
			hook: func(dir string) func(string) error {
				return at(stepStaged, func() { _ = os.Chmod(dir, 0o500) })
			},
		},
		{
			name: "the new conflicts cannot be moved into place",
			hook: func(dir string) func(string) error {
				return at(stepStaged, func() { _ = os.RemoveAll(filepath.Join(dir, stageName)) })
			},
		},
		{
			name: "the old conflicts cannot be removed",
			prep: func(dir string) { _ = os.Chmod(filepath.Join(dir, conflictsDir, "aaa111"), 0o500) },
		},
		{
			name: "the state cannot be moved into place",
			hook: func(dir string) func(string) error {
				return at(stepWritten, func() {
					temps, _ := filepath.Glob(filepath.Join(dir, tmpPrefix+"*"))
					for _, temp := range temps {
						_ = os.Remove(temp)
					}
				})
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if os.Geteuid() == 0 {
				t.Skip("root ignores directory permissions")
			}

			dir := filepath.Join(t.TempDir(), "state")
			if err := Save(dir, stateOf("", "aaa111")); err != nil {
				t.Fatalf("first Save() = %v", err)
			}

			var hook func(string) error

			if testCase.hook != nil {
				hook = testCase.hook(dir)
			}

			if testCase.prep != nil {
				testCase.prep(dir)
			}

			checkRecovery(t, dir, hook)
		})
	}
}

func checkRecovery(t *testing.T, dir string, hook func(string) error) {
	t.Helper()

	free := func() {
		for _, path := range []string{
			dir, filepath.Join(dir, conflictsDir, "aaa111"), filepath.Join(dir, oldName, "aaa111"),
		} {
			_ = os.Chmod(path, 0o700)
		}
	}
	t.Cleanup(free)

	if err := (saver{hook: hook}).save(dir, stateOf("", "bbb222")); err == nil {
		t.Fatal("save() = nil, want the failure")
	}

	free()

	if state, err := Load(dir); err != nil || state.Conflicts[0].Rev != "aaa111" {
		t.Fatalf("after the failure Load() = %+v, %v, want the old state", state, err)
	}

	if err := Save(dir, stateOf("", "bbb222")); err != nil {
		t.Fatalf("Save() after the failure = %v, want the way forward", err)
	}

	assertNoLeftovers(t, dir)
}

func TestSaveRollsTheOldConflictsBack(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "state")
	if err := Save(dir, stateOf("", "aaa111")); err != nil {
		t.Fatalf("first Save() = %v", err)
	}

	hook := at(stepStaged, func() { _ = os.RemoveAll(filepath.Join(dir, stageName)) })

	if err := (saver{hook: hook}).save(dir, stateOf("", "bbb222")); err == nil {
		t.Fatal("save() = nil, want the failure")
	}

	for name, want := range map[string]string{
		"aaa111/00-Run.final": "func Run() { Z() }",
		markerFile:            "aaa111/00-Run.final\naaa111/00-Run.preimage\n",
	} {
		got, err := os.ReadFile(filepath.Join(dir, conflictsDir, filepath.FromSlash(name)))
		if err != nil || string(got) != want {
			t.Fatalf("%s = %q, %v, want the old conflicts back: %q", name, got, err, want)
		}
	}

	if _, err := os.Stat(filepath.Join(dir, oldName)); err == nil {
		t.Fatal("the set-aside conflicts were left beside the restored ones")
	}
}

func TestSaveKill(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		step         string
		wantNewTexts bool
		wantLeftover bool
	}{
		{name: "a kill after the texts are staged", step: stepStaged, wantLeftover: true},
		{name: "a kill after the texts are swapped in", step: stepSwapped, wantNewTexts: true},
		{
			name: "a kill after the state is written and before it is renamed", step: stepWritten,
			wantNewTexts: true, wantLeftover: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := filepath.Join(t.TempDir(), "state")
			if err := Save(dir, stateOf(anchoring.PolicyWIP, "aaa111")); err != nil {
				t.Fatalf("first Save() = %v", err)
			}

			err := saver{hook: killAt(testCase.step)}.save(
				dir, stateOf(anchoring.PolicyFold, "bbb222"),
			)
			if !errors.Is(err, errKilled) {
				t.Fatalf("save() = %v, want the kill", err)
			}

			state, loadErr := Load(dir)
			if loadErr != nil || state.Decisions["decided"] != anchoring.PolicyWIP ||
				state.Conflicts[0].Rev != "aaa111" {
				t.Fatalf("after the kill Load() = %+v, %v, want the old state", state, loadErr)
			}

			_, newErr := os.Stat(filepath.Join(dir, "conflicts", "bbb222"))
			if (newErr == nil) != testCase.wantNewTexts {
				t.Fatalf("new texts present = %v, want %v", newErr == nil, testCase.wantNewTexts)
			}

			if left := len(leftovers(t, dir)) > 0; left != testCase.wantLeftover {
				t.Fatalf("leftovers = %v, want %v", leftovers(t, dir), testCase.wantLeftover)
			}

			if err = Save(dir, stateOf(anchoring.PolicyFold, "bbb222")); err != nil {
				t.Fatalf("Save() after the kill = %v", err)
			}

			assertNoLeftovers(t, dir)
		})
	}
}

func TestSaveKillOfTheFirstSave(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "state")

	err := saver{hook: killAt(stepSwapped)}.save(dir, stateOf("", "aaa111"))
	if !errors.Is(err, errKilled) {
		t.Fatalf("save() = %v, want the kill", err)
	}

	if _, loadErr := Load(dir); !errors.Is(loadErr, ErrNoState) {
		t.Fatalf("Load() = %v, want ErrNoState", loadErr)
	}

	if err = Remove(dir); err != nil {
		t.Fatalf("Remove() = %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(dir, "conflicts")); statErr == nil {
		t.Fatal("the abort left the marked conflicts")
	}
}

func TestSaveWithATextThatCannotBeStagedKeepsTheOldState(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "state")
	if err := Save(dir, stateOf("", "aaa111")); err != nil {
		t.Fatalf("first Save() = %v", err)
	}

	state := stateOf("", "bbb222")
	state.Conflicts[0].Decls[0].Key.Name = "a\x00b"

	if err := Save(dir, state); err == nil {
		t.Fatal("Save() = nil, want the refusal")
	}

	if loaded, err := Load(dir); err != nil || loaded.Conflicts[0].Rev != "aaa111" {
		t.Fatalf("Load() = %+v, %v, want the old state", loaded, err)
	}
}

func TestStageNamesEveryTextBeforeWritingOne(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{stageName + "/bbb222": "in the way"})

	if err := (saver{}).stage(dir, stateOf("", "bbb222")); err == nil {
		t.Fatal("stage() = nil, want the failure")
	}

	listed, hasMarker, err := readMarker(filepath.Join(dir, stageName))
	want := []string{"bbb222/00-Run.final", "bbb222/00-Run.preimage"}

	if err != nil || !hasMarker || !slices.Equal(listed, want) {
		t.Fatalf("readMarker() = %q, %v, %v, want %q", listed, hasMarker, err, want)
	}
}

func TestSaveKeepsAFileThatSlippedInAfterTheCheck(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		slipped string
	}{
		{name: "beside the rev directories", slipped: "slipped"},
		{name: "inside a rev directory", slipped: "aaa111/slipped"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := filepath.Join(t.TempDir(), "state")
			if err := Save(dir, stateOf("", "aaa111")); err != nil {
				t.Fatalf("first Save() = %v", err)
			}

			path := filepath.Join(dir, oldName, filepath.FromSlash(testCase.slipped))
			hook := at(stepStaged, func() {
				_ = os.WriteFile(
					filepath.Join(dir, conflictsDir, filepath.FromSlash(testCase.slipped)),
					[]byte("mine"), 0o600,
				)
			})

			if err := (saver{hook: hook}).save(dir, stateOf("", "bbb222")); err != nil {
				t.Fatalf("save() = %v, want the new state in place", err)
			}

			for pass := 1; pass <= 2; pass++ {
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("pass %d: the slipped file did not survive: %v", pass, err)
				}

				if err := Save(dir, stateOf("", "ccc333")); !errors.Is(err, ErrForeign) {
					t.Fatalf("pass %d: Save() = %v, want ErrForeign", pass, err)
				}
			}
		})
	}
}
