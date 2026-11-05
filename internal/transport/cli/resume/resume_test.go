package resume

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
	"github.com/wrapped-owls/redigit/internal/infra/anchorstate"
	"github.com/wrapped-owls/redigit/internal/infra/lineageconf"
)

const (
	firstRev  = "3d2d883000000000000000000000000000000000"
	secondRev = "0aab4c6000000000000000000000000000000000"
	thirdRev  = "9f8e7d6000000000000000000000000000000000"

	stopConfig = `{"lineage": {"policy": "stop", "anchors": [
		{"rev": "3d2d883", "kind": "fix"}, {"rev": "0aab4c6", "kind": "perf"}]}}`
)

func testFlags() *flag.FlagSet {
	set := flag.NewFlagSet("test", flag.ContinueOnError)
	set.SetOutput(io.Discard)

	for _, name := range []string{"repo", "config", "state-dir", "tags"} {
		set.String(name, "", "")
	}

	set.String("rev", "HEAD", "")
	set.Bool("timings", false, "")
	set.Bool("from-cache", false, "")

	return set
}

type recorder struct {
	argv [][]string
	err  error
}

func (fake *recorder) dispatch(_ context.Context, args []string, stdout, _ io.Writer) error {
	fake.argv = append(fake.argv, slices.Clone(args))
	_, _ = stdout.Write([]byte("ran"))

	return fake.err
}

func conflictsAt(name string, revs ...string) []anchoring.Conflict {
	key := entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: name}
	decls := []anchoring.ConflictDecl{{Key: key, Preimage: "old", Final: "new"}}

	conflicts := make([]anchoring.Conflict, 0, len(revs))

	for _, rev := range revs {
		conflicts = append(
			conflicts,
			anchoring.Conflict{Rev: rev, Detail: "undefined: Z", Decls: decls},
		)
	}

	return conflicts
}

func stoppedAt(name string, revs ...string) error {
	return &anchoring.StoppedError{Conflicts: conflictsAt(name, revs...)}
}

func stopError() error { return stoppedAt("Run", firstRev) }

func savedState(t *testing.T, content string, revs ...string) (dir, config string) {
	t.Helper()

	dir = filepath.Join(t.TempDir(), "state")
	config = filepath.Join(filepath.Dir(dir), "cfg.json")

	if err := os.WriteFile(config, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	digest, err := anchorstate.DigestFile(config)
	if err != nil {
		t.Fatal(err)
	}

	state := anchorstate.State{
		Args:   []string{"emit", "--repo", "R", "--config", config, "--state-dir", dir},
		Commit: "c1", Digest: digest, Conflicts: conflictsAt("Run", revs...),
	}

	if err = anchorstate.Save(dir, state); err != nil {
		t.Fatal(err)
	}

	return dir, config
}

func stoppedState(t *testing.T) (dir, config string) {
	t.Helper()

	return savedState(t, stopConfig, firstRev, secondRev)
}

func derivedSection(t *testing.T, dir string) lineageconf.Section {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(dir, anchorstate.DerivedFile))
	if err != nil {
		t.Fatal(err)
	}

	var file map[string]json.RawMessage
	if err = json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}

	section, err := lineageconf.Decode(file["lineage"])
	if err != nil {
		t.Fatal(err)
	}

	return section
}

func inDir(args []string, dir string) []string {
	out := make([]string, len(args))
	for idx, arg := range args {
		out[idx] = strings.ReplaceAll(arg, "{dir}", dir)
	}

	return out
}

func writeAll(t *testing.T, dir string, files map[string]string) {
	t.Helper()

	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))

		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func lock(t *testing.T, path string) {
	t.Helper()

	if os.Geteuid() == 0 {
		t.Skip("the owner of the directory ignores its mode")
	}

	if err := os.Chmod(path, 0o500); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.Chmod(path, 0o700) })
}
