package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func piece(file, name string, span entities.Span) entities.Decl {
	return entities.Decl{
		Key:  entities.DeclKey{File: file, Kind: entities.KindOther, Name: name},
		Unit: entities.UnitID(filepath.Dir(file)),
		Span: span,
	}
}

// treeWith writes files into a temporary root and returns the Snapshot a rule
// reads them through.
func treeWith(t *testing.T, files map[string]string) entities.Snapshot {
	t.Helper()

	root := t.TempDir()

	paths := make([]string, 0, len(files))

	for name, body := range files {
		full := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}

		paths = append(paths, name)
	}

	return entities.Snapshot{Root: root, Files: paths}
}

func edgeBetween(edges []entities.Edge, dependent, provider entities.DeclKey) bool {
	for _, edge := range edges {
		if edge.Dependent == dependent && edge.Provider == provider {
			return true
		}
	}

	return false
}

// The restriction that broke 53 trees on a real repository: Go reads a
// replaced directory even when nothing imports the module, so a replace line
// born before its target's go.mod breaks every build until the target arrives.
func TestWorkspaceRuleTiesAReplaceToItsTargetModule(t *testing.T) {
	t.Parallel()

	const appMod = "apps/lead/go.mod"

	body := "module github.com/org/app\n\ngo 1.26\n\n" +
		"require github.com/org/lib v0.0.0\n\n" +
		"replace github.com/org/lib => ../../libs/lib\n"

	snap := treeWith(t, map[string]string{appMod: body})

	header := piece(appMod, "github.com/org/app", entities.Span{Start: 0, End: 30})
	replace := piece(
		appMod,
		"replace github.com/org/lib",
		entities.Span{Start: len(body) - 44, End: len(body)},
	)
	libHeader := piece("libs/lib/go.mod", "github.com/org/lib", entities.Span{Start: 0, End: 30})

	edges, err := (workspaceRule{}).Edges(
		t.Context(), snap, []entities.Decl{header, replace, libHeader},
	)
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	if !edgeBetween(edges, replace.Key, libHeader.Key) {
		t.Fatalf(
			"the replace line does not wait for %s: it would land before the directory exists "+
				"and break every build until it does; got %d edge(s)",
			libHeader.Key.File, len(edges),
		)
	}
}

func TestWorkspaceRuleTiesAUseLineToItsModule(t *testing.T) {
	t.Parallel()

	snap := treeWith(t, map[string]string{"go.work": "go 1.26\n\nuse ./modules/lead\n"})

	use := piece("go.work", "use ./modules/lead", entities.Span{Start: 10, End: 29})
	header := piece("modules/lead/go.mod", "github.com/org/lead", entities.Span{Start: 0, End: 30})

	edges, err := (workspaceRule{}).Edges(t.Context(), snap, []entities.Decl{use, header})
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	if !edgeBetween(edges, use.Key, header.Key) {
		t.Fatalf("the use line does not wait for the module it names; got %d edge(s)", len(edges))
	}
}

// A replace whose right-hand side is a version, not a path, names no directory
// in this tree: nothing has to exist for it, and inventing an edge would order
// it against a module that is not there.
func TestWorkspaceRuleIgnoresAVersionOnlyReplace(t *testing.T) {
	t.Parallel()

	const modFile = "go.mod"

	body := "module github.com/org/app\n\nreplace github.com/x/y v1.0.0 => github.com/x/y v1.1.0\n"
	snap := treeWith(t, map[string]string{modFile: body})

	replace := piece(modFile, "replace github.com/x/y", entities.Span{Start: 27, End: len(body)})
	header := piece(modFile, "github.com/org/app", entities.Span{Start: 0, End: 27})

	edges, err := (workspaceRule{}).Edges(t.Context(), snap, []entities.Decl{replace, header})
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	if len(edges) != 0 {
		t.Fatalf("a version-only replace produced %d edge(s), want none: %v", len(edges), edges)
	}
}

// A replace pointing outside the repository has no module piece to wait for.
// No edge is the honest answer; an edge to nothing would pin the line at
// whatever the dangling guard decides.
func TestWorkspaceRuleIgnoresATargetTheTreeDoesNotHold(t *testing.T) {
	t.Parallel()

	const modFile = "go.mod"

	body := "module github.com/org/app\n\nreplace github.com/x/y => ../outside\n"
	snap := treeWith(t, map[string]string{modFile: body})

	replace := piece(modFile, "replace github.com/x/y", entities.Span{Start: 27, End: len(body)})
	header := piece(modFile, "github.com/org/app", entities.Span{Start: 0, End: 27})

	edges, err := (workspaceRule{}).Edges(t.Context(), snap, []entities.Decl{replace, header})
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	if len(edges) != 0 {
		t.Fatalf("got %d edge(s) for a target the tree does not hold: %v", len(edges), edges)
	}
}

// An unreadable file must not fail the index: one missing replace edge orders
// one line by history alone, while failing the run loses every other edge.
func TestWorkspaceRuleSurvivesAnUnreadableFile(t *testing.T) {
	t.Parallel()

	snap := entities.Snapshot{Root: t.TempDir(), Files: []string{"go.mod"}}

	replace := piece("go.mod", "replace github.com/x/y", entities.Span{Start: 0, End: 40})
	header := piece("go.mod", "github.com/org/app", entities.Span{Start: 0, End: 0})

	edges, err := (workspaceRule{}).Edges(t.Context(), snap, []entities.Decl{replace, header})
	if err != nil {
		t.Fatalf("Edges() = %v, want the run to continue without this edge", err)
	}

	if len(edges) != 0 {
		t.Fatalf("got %d edge(s) from a file that could not be read", len(edges))
	}
}

// A require whose module is replaced locally carries the pseudo-version
// v0.0.0-00010101000000-000000000000, which means "the replace resolves this".
// With the target directory not there yet, the go command tries the network
// for that version and fails - 247 trees on a real repository.
func TestWorkspaceRuleMakesARequireWaitForItsLocalReplace(t *testing.T) {
	t.Parallel()

	const appMod = "apps/lead/go.mod"

	body := "module github.com/org/app\n\ngo 1.26\n\n" +
		"require github.com/org/lib v0.0.0-00010101000000-000000000000\n\n" +
		"replace github.com/org/lib => ../../libs/lib\n"

	snap := treeWith(t, map[string]string{appMod: body})

	header := piece(appMod, "github.com/org/app", entities.Span{Start: 0, End: 36})
	require := piece(appMod, "require github.com/org/lib", entities.Span{Start: 36, End: 98})
	replace := piece(appMod, "replace github.com/org/lib", entities.Span{Start: 98, End: len(body)})
	libHeader := piece("libs/lib/go.mod", "github.com/org/lib", entities.Span{Start: 0, End: 30})

	edges, err := (workspaceRule{}).Edges(
		t.Context(), snap, []entities.Decl{header, require, replace, libHeader},
	)
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	if !edgeBetween(edges, require.Key, libHeader.Key) {
		t.Fatalf(
			"the require does not wait for %s: the replace resolves it, and without the "+
				"target the go command reaches the network for a version that does not exist",
			libHeader.Key.File,
		)
	}
}

// A require with no local replace resolves from the module proxy and needs
// nothing in this tree.
func TestWorkspaceRuleLeavesAnOrdinaryRequireAlone(t *testing.T) {
	t.Parallel()

	const modFile = "go.mod"

	body := "module github.com/org/app\n\nrequire github.com/x/y v1.2.3\n"
	snap := treeWith(t, map[string]string{modFile: body})

	header := piece(modFile, "github.com/org/app", entities.Span{Start: 0, End: 27})
	require := piece(modFile, "require github.com/x/y", entities.Span{Start: 27, End: len(body)})
	other := piece("libs/lib/go.mod", "github.com/x/y", entities.Span{Start: 0, End: 20})

	edges, err := (workspaceRule{}).Edges(
		t.Context(), snap, []entities.Decl{header, require, other},
	)
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	if len(edges) != 0 {
		t.Fatalf("got %d edge(s) for a require nothing replaces: %v", len(edges), edges)
	}
}
