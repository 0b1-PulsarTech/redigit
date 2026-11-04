//go:build integration

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const constFix = "fix: add KeyC"

func newConstGroupSource(t *testing.T, source, keys, fixKeys string) {
	t.Helper()

	if err := os.MkdirAll(source, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	gitText(t, source, "init", "-q", "-b", "main")
	gitText(t, source, "config", "user.email", "tester@example.com")
	gitText(t, source, "config", "user.name", "Tester")

	files := map[string]string{
		"go.mod":   modFile,
		"types.go": "package p\n\ntype Key int\n",
		"keys.go":  keys,
		"c24.go":   "package p\n\nconst C24 = 1\n",
	}

	for count := 1; count < 24; count++ {
		files[fmt.Sprintf("c%02d.go", count)] = fmt.Sprintf(
			"package p\n\nconst C%02d = C%02d + 1\n", count, count+1)
	}

	commitFiles(t, source, "init", files)

	if fixKeys != "" {
		commitFiles(t, source, constFix, map[string]string{"keys.go": fixKeys})
	}
}

func writeConstGroup(t *testing.T, scratch, source, bare, policy string) {
	t.Helper()

	if policy == "" {
		writeAndVerify(t, scratch, source, bare)

		return
	}

	rev := gitText(t, source, "log", "--format=%H", "--fixed-strings", "--grep="+constFix, "-1")
	config := filepath.Join(scratch, "cfg.json")
	identity := `{"write":{"identity":{"name":"proof","email":"proof@localhost"}}`
	settings := identity + `,"lineage":{"policy":"` + policy +
		`","anchors":[{"rev":"` + rev + `","kind":"fix"}]}}`

	if err := os.WriteFile(config, []byte(settings), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	blobs := filepath.Join(scratch, "blobs")
	manifest := filepath.Join(scratch, "emit.json")
	emitted := runCLI(t, "emit", "--repo", source, "--mode", "lineage", "--config", config,
		"--blob-dir", blobs)

	if err := os.WriteFile(manifest, []byte(emitted), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	runCLI(t, "write", "--repo", bare, "--manifest", manifest, "--ref", "refs/heads/proof",
		"--date-ref", "HEAD", "--verify", "HEAD", "--config", config, "--allow-draft",
		"--blob-dir", blobs)
	runCLI(t, "verify", "--repo", bare, "--ref", "refs/heads/proof",
		"--cache-dir", filepath.Join(scratch, "verify"), "--config", config)
}

func assertRowsBornOnce(t *testing.T, bare string, wantTouches int, wantMembers []string) {
	t.Helper()

	revs := strings.Fields(gitText(t, bare, "rev-list", "--reverse", "refs/heads/proof"))
	for _, rev := range revs[1:] {
		tree := gitText(t, bare, "rev-parse", rev+"^{tree}")
		if tree == gitText(t, bare, "rev-parse", rev+"~1^{tree}") {
			t.Fatalf("row %s holds the tree of its parent", rev)
		}
	}

	touching := strings.Fields(gitText(t, bare, "log", "--reverse", "--format=%H",
		"refs/heads/proof", "--", "keys.go"))
	if len(touching) != wantTouches {
		t.Fatalf("keys.go is written by %d rows, want %d", len(touching), wantTouches)
	}

	born := gitText(t, bare, "show", touching[0]+":keys.go")
	for _, member := range wantMembers {
		if !strings.Contains(born, member) {
			t.Fatalf("the row that first writes keys.go lacks %s:\n%s", member, born)
		}
	}

	if !strings.Contains(gitText(t, bare, "ls-tree", "--name-only", touching[0]), "c01.go") {
		t.Fatalf("the tree of the row that first writes keys.go lacks the c01.go it reads")
	}
}

type constGroupCase struct {
	name        string
	policy      string
	keys        string
	fixKeys     string
	wantTouches int
	wantMembers []string
}

func runConstGroupCases(t *testing.T, testCases []constGroupCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			scratch := t.TempDir()
			source := filepath.Join(scratch, "source")
			bare := filepath.Join(scratch, "bare")

			newConstGroupSource(t, source, testCase.keys, testCase.fixKeys)
			gitText(t, scratch, "clone", "-q", "--bare", "--no-local", source, bare)
			writeConstGroup(t, scratch, source, bare, testCase.policy)
			assertRowsBornOnce(t, bare, testCase.wantTouches, testCase.wantMembers)

			if gitText(t, bare, "rev-parse", "refs/heads/proof^{tree}") !=
				gitText(t, source, "rev-parse", "HEAD^{tree}") {
				t.Fatal("the written tree differs from the source tree")
			}
		})
	}
}

func TestImplicitConstGroupIsBornInOneRow(t *testing.T) {
	const (
		keysAB = "package p\n\nconst (\n\tKeyA Key = iota + C01\n\tKeyB\n)\n"
		keysAC = "package p\n\nconst (\n\tKeyA Key = iota + C01\n\tKeyB\n\tKeyC\n)\n"
	)

	testCases := []constGroupCase{
		{
			name:        "a plain emission",
			keys:        keysAC,
			wantTouches: 1,
			wantMembers: []string{"KeyA", "KeyB", "KeyC"},
		},
		{
			name:        "a lineage fold over a fix that adds KeyC to the block",
			policy:      "fold",
			keys:        keysAB,
			fixKeys:     keysAC,
			wantTouches: 2,
			wantMembers: []string{"KeyA", "KeyB"},
		},
		{
			name:        "a lineage wip over a fix that adds KeyC to the block",
			policy:      "wip",
			keys:        keysAB,
			fixKeys:     keysAC,
			wantTouches: 2,
			wantMembers: []string{"KeyA", "KeyB"},
		},
	}

	runConstGroupCases(t, testCases)
}

func TestValueSpecsAreBornInOneRow(t *testing.T) {
	const (
		multAB = "package p\n\nconst KeyA, KeyB = 1, C01 + 1\n"
		multAC = "package p\n\nconst KeyA, KeyB, KeyC = 1, C01 + 1, 3\n"
	)

	testCases := []constGroupCase{
		{
			name:        "a const spec binding two names",
			keys:        multAB,
			wantTouches: 1,
			wantMembers: []string{"KeyA", "KeyB"},
		},
		{
			name:        "a lineage fold over a fix that adds a third name to the spec",
			policy:      "fold",
			keys:        multAB,
			fixKeys:     multAC,
			wantTouches: 2,
			wantMembers: []string{"KeyA", "KeyB"},
		},
		{
			name:        "a var spec binding two names",
			keys:        "package p\n\nvar VarA, VarB = 1, C01 + 1\n\nfunc Use() int { return VarA }\n",
			wantTouches: 1,
			wantMembers: []string{"VarA", "VarB"},
		},
		{
			name:        "a documented multi-name spec in a block",
			keys:        "package p\n\nconst (\n\t// KeyA doc\n\tKeyA, KeyB = 1, C01\n)\n",
			wantTouches: 1,
			wantMembers: []string{"KeyA", "KeyB"},
		},
		{
			name:        "explicit specs where a later one uses iota",
			keys:        "package p\n\nconst (\n\tKeyA = iota + C01\n\tKeyB = iota\n)\n",
			wantTouches: 1,
			wantMembers: []string{"KeyA", "KeyB"},
		},
	}

	runConstGroupCases(t, testCases)
}
