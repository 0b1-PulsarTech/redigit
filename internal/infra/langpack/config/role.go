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
	// roleProse is born with the first code it describes: a document naming
	// a package, a README beside one.
	roleProse
)

// placement is the role of one file and, for prose, the code it describes.
type placement struct {
	role  role
	homes []entities.DeclKey
}

// placements maps a whole-file declaration to where its path puts it.
type placements map[entities.DeclKey]placement

// classify gives every whole-file declaration the role its path plays in the
// repository, never a file list of one repository.
//
// A file no rule places is an orphan: nothing reads it, so the planner has no
// feature to birth it in and leaves it for the trailing "reached by no entry
// point" commit. On a real repository that sent .gitignore, the devcontainer,
// every Taskfile and the lint configuration to the last 1.5% of the history.
func classify(snap entities.Snapshot, decls []entities.Decl) placements {
	headers := moduleHeaders(decls)
	roots := slices.Sorted(maps.Keys(headers))
	classifier := classifier{
		roots: roots,
		coded: codedModules(roots, decls),
		code:  newCodeIndex(snap, headers, decls),
		files: map[string]placement{},
	}
	out := placements{}

	for _, decl := range decls {
		if !isWholeFile(decl.Key) || isInert(decl.Key.File) {
			continue
		}

		out[decl.Key] = classifier.place(decl.Key.File)
	}

	return out
}

type classifier struct {
	roots []string
	coded map[string]bool
	code  codeIndex
	files map[string]placement
}

// place decides one file, once: the pieces of a long file share its role. A
// module's own file waits for its module, but a module with no code has no
// first package to be born with, so its files are scaffold: nothing would
// ever pull them out of the trailing commit.
func (c classifier) place(file string) placement {
	if found, done := c.files[file]; done {
		return found
	}

	decided := c.decide(file)
	c.files[file] = decided

	return decided
}

func (c classifier) decide(file string) placement {
	if isScaffoldPath(file) {
		return placement{role: roleScaffold}
	}

	root, found := nearestModule(c.roots, file)
	if found && path.Dir(file) == root {
		if c.coded[root] {
			return placement{role: roleModuleOwn}
		}

		return placement{role: roleScaffold}
	}

	if !isProse(file) {
		return placement{role: roleNone}
	}

	if homes := c.code.describedBy(file); len(homes) > 0 {
		return placement{role: roleProse, homes: homes}
	}

	return placement{role: roleScaffold}
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
