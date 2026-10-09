package config

import (
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// wholeFilePrefix is the name chunk.WholeFile gives the declaration standing
// for a file the tool does not read: "<file>", and "<file>#2" for the second
// run of lines of a long one.
const wholeFilePrefix = "<file>"

// role says where a whole-file declaration is born.
type role int

const (
	// roleNone leaves the file to the edges its own format already has.
	roleNone role = iota
	// roleScaffold is born before the first module: the repository's own
	// dotfiles, editor and lint configuration, root documents.
	roleScaffold
	// roleModuleOwn is born with the go.mod of the module whose root holds it:
	// its README, Taskfile, configuration samples.
	roleModuleOwn
)

// placements maps a whole-file declaration to the role its path gives it.
type placements map[entities.DeclKey]role

// classify gives every whole-file declaration the role its path plays in the
// repository, never a file list of one repository.
//
// A file no rule places is an orphan: nothing reads it, so the planner has no
// feature to birth it in and leaves it for the trailing "reached by no entry
// point" commit. On a real repository that sent .gitignore, the devcontainer,
// every Taskfile and the lint configuration to the last 1.5% of the history.
func classify(decls []entities.Decl) placements {
	out := placements{}
	roots := slices.Sorted(maps.Keys(moduleHeaders(decls)))
	coded := codedModules(roots, decls)

	for _, decl := range decls {
		if !isWholeFile(decl.Key) || isInert(decl.Key.File) {
			continue
		}

		out[decl.Key] = place(decl.Key.File, roots, coded)
	}

	return out
}

// place decides one file. A module's own file waits for its module, but a
// module with no code has no first package to be born with, so its files are
// scaffold: nothing would ever pull them out of the trailing commit.
func place(file string, roots []string, coded map[string]bool) role {
	if isScaffoldPath(file) {
		return roleScaffold
	}

	root, found := nearestModule(roots, file)
	if !found || path.Dir(file) != root {
		return roleNone
	}

	if coded[root] {
		return roleModuleOwn
	}

	return roleScaffold
}

// codedModules marks the modules holding a declaration of a format the tool
// reads, which is what a README can be born beside.
func codedModules(roots []string, decls []entities.Decl) map[string]bool {
	out := map[string]bool{}

	for _, decl := range decls {
		if decl.Key.Kind == entities.KindOther {
			continue
		}

		if root, found := nearestModule(roots, decl.Key.File); found {
			out[root] = true
		}
	}

	return out
}

func isWholeFile(key entities.DeclKey) bool {
	return key.Kind == entities.KindOther && strings.HasPrefix(key.Name, wholeFilePrefix)
}

// inertDirs hold what a repository writes last or by hand with an owner of
// its own: pipelines and agent instructions are the final commits' business.
var inertDirs = []string{".github", ".gitlab", ".gitea", ".circleci", ".claude"}

// isInert reports a file no scaffold or documentation rule may move: the
// formats other rules own (Go, SQL, go.mod, go.sum, go.work), fixtures a test
// reads, and the pipeline directories.
func isInert(file string) bool {
	switch path.Ext(file) {
	case ".go", ".sql":
		return true
	}

	switch path.Base(file) {
	case goModFile, goSumFile, goWorkFile:
		return true
	}

	segments := strings.Split(file, "/")

	return slices.Contains(segments, "testdata") || slices.Contains(inertDirs, segments[0])
}

// isScaffoldPath recognises the files that describe the repository rather than
// any one module: anything at the root, anything under a root dot-directory
// (.idea, .devcontainer), and a linter configuration wherever it sits.
func isScaffoldPath(file string) bool {
	if path.Dir(file) == "." || strings.HasPrefix(file, ".") {
		return true
	}

	return strings.HasPrefix(path.Base(file), ".golangci.")
}
