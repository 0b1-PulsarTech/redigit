package anchorstate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
)

var errKilled = errors.New("killed")

func conflictFor(rev string) anchoring.Conflict {
	key := entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: "Run"}
	decl := anchoring.ConflictDecl{Key: key, Preimage: "func Run() {}", Final: "func Run() { Z() }"}

	return anchoring.Conflict{
		Rev:    rev,
		Detail: "undefined: Z",
		Decls:  []anchoring.ConflictDecl{decl},
	}
}

func killAt(step string) func(string) error {
	return func(reached string) error {
		if reached == step {
			return errKilled
		}

		return nil
	}
}

func stateOf(decision anchoring.Policy, revs ...string) State {
	state := State{Args: []string{"emit", "--repo", "r"}, Commit: "c1", Digest: "d1"}
	if decision != "" {
		state.Decisions = map[string]anchoring.Policy{"decided": decision}
	}

	for _, rev := range revs {
		state.Conflicts = append(state.Conflicts, conflictFor(rev))
	}

	return state
}

func lock(t *testing.T, path string) {
	t.Helper()

	if err := os.Chmod(path, 0o500); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.Chmod(path, 0o700) })
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()

	if err := os.MkdirAll(dir, dirMode); err != nil {
		t.Fatal(err)
	}

	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))

		if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func writeLayout(t *testing.T, dir string, dirs []string, links map[string]string) {
	t.Helper()

	if err := os.MkdirAll(dir, dirMode); err != nil {
		t.Fatal(err)
	}

	for _, name := range dirs {
		if err := os.MkdirAll(filepath.Join(dir, name), dirMode); err != nil {
			t.Fatal(err)
		}
	}

	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
}

func snapshot(t *testing.T, dir string) string {
	t.Helper()

	var out strings.Builder

	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		body := ""

		if entry.Type()&fs.ModeSymlink != 0 {
			target, linkErr := os.Readlink(path)
			if linkErr != nil {
				return fmt.Errorf("read a link: %w", linkErr)
			}

			body = "-> " + target
		} else if !entry.IsDir() {
			raw, readErr := os.ReadFile(path)
			if readErr != nil {
				return fmt.Errorf("read a file: %w", readErr)
			}

			body = string(raw)
		}

		out.WriteString(path + "=" + body + "\n")

		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}

	return out.String()
}

func leftovers(t *testing.T, dir string) []string {
	t.Helper()

	var found []string

	tmpName := regexp.MustCompile(`^\.redigit-tmp-[0-9]+$`)

	err := filepath.WalkDir(dir, func(path string, _ fs.DirEntry, _ error) error {
		base := filepath.Base(path)

		if base == stageName || base == oldName || tmpName.MatchString(base) {
			found = append(found, path)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	return found
}

func assertNoLeftovers(t *testing.T, dir string) {
	t.Helper()

	if found := leftovers(t, dir); len(found) > 0 {
		t.Errorf("redigit leftovers remain: %v", found)
	}
}
