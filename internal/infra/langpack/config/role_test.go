package config

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestClassifyPlacesScaffoldByPathRole(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		file string
		want role
	}{
		{"a root dotfile", ".gitignore", roleScaffold},
		{"a root build file", "Taskfile.yml", roleScaffold},
		{"a root agent instruction stays last", "AGENTS.md", roleNone},
		{"a second agent instruction stays last", "CLAUDE.md", roleNone},
		{"an agent rules dotfile stays last", ".pulsarules-languages.json", roleNone},
		{"a rules document naming nothing is left for the end", "docs/SERVICE_FLOW.md", roleNone},
		{"an editor directory", ".idea/vcs.xml", roleScaffold},
		{"a nested editor file", ".devcontainer/features/go.json", roleScaffold},
		{"a lint config inside a module", "tools/.golangci.yml", roleScaffold},
		{"a module's own build file", "database/Taskfile.yml", roleNone},
		{"a pipeline stays last", ".github/workflows/ci.yml", roleNone},
		{"an agent directory stays last", ".claude/settings.json", roleNone},
		{"a fixture a test reads", "testdata/.golangci.yml", roleNone},
		{"a Go file that declares nothing", "doc.go", roleNone},
		{"a manifest has its own rules", "go.work", roleNone},
		{"a checksum file has its own rules", "go.sum", roleNone},
		{"a migration has its own rules", "schema.sql", roleNone},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			decl := piece(testCase.file, wholeFilePrefix, entities.Span{})

			if got := classify(entities.Snapshot{}, []entities.Decl{decl})[decl.Key].role; got != testCase.want {
				t.Errorf("classify(%s) = %v, want %v", testCase.file, got, testCase.want)
			}
		})
	}
}

func TestClassifyLeavesPiecesOfAFormatAlone(t *testing.T) {
	t.Parallel()

	require := piece(".idea/go.mod", "require example.com/a", entities.Span{})

	if got := classify(entities.Snapshot{}, []entities.Decl{require})[require.Key].role; got != roleNone {
		t.Errorf("a go.mod piece is placed as %v", got)
	}
}

func TestClassifyPlacesEveryPieceOfALongFile(t *testing.T) {
	t.Parallel()

	first := piece("CHANGELOG.md", "<file>", entities.Span{})
	second := piece("CHANGELOG.md", "<file>#2", entities.Span{})

	got := classify(entities.Snapshot{}, []entities.Decl{first, second})

	if got[first.Key].role != got[second.Key].role || got[first.Key].role != roleNone {
		t.Errorf("classify = %v, want both pieces placed alike", got)
	}
}

func TestClassifyPlacesAModulesRootFiles(t *testing.T) {
	t.Parallel()

	appHeader := piece("apps/lead/go.mod", "example.com/app", entities.Span{})
	appCode := goDecl("apps/lead/svc/svc.go", "Serve")
	toolsHeader := piece("tools/go.mod", "example.com/tools", entities.Span{})
	decls := []entities.Decl{appHeader, appCode, toolsHeader}

	testCases := []struct {
		name string
		file string
		want role
	}{
		{"a module's README", "apps/lead/README.md", roleModuleOwn},
		{"a module's build file", "apps/lead/Taskfile.yml", roleModuleOwn},
		{"a note beside the code it describes", "apps/lead/svc/notes.md", roleProse},
		{"a nested file no format claims", "apps/lead/svc/data.bin", roleNone},
		{"a file of a module with no code", "tools/Taskfile.yml", roleScaffold},
		{"a file outside every module", "build/Taskfile.build.yml", roleNone},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			file := piece(testCase.file, wholeFilePrefix, entities.Span{})

			got := classify(entities.Snapshot{}, append([]entities.Decl{file}, decls...))[file.Key].role
			if got != testCase.want {
				t.Errorf("classify(%s) = %v, want %v", testCase.file, got, testCase.want)
			}
		})
	}
}

func TestClassifyPlacesAnyDocumentNamingCodeAfterIt(t *testing.T) {
	t.Parallel()

	snap := treeWith(t, map[string]string{
		"README.md":       "Run apps/lead/svc first.\n",
		"docs/guide.md":   "See apps/lead/svc.\n",
		"docs/nothing.md": "Hello.\n",
	})

	code := goDecl("apps/lead/svc/svc.go", "Serve")
	header := piece("apps/lead/go.mod", "example.com/app", entities.Span{})

	testCases := []struct {
		name string
		file string
		want role
	}{
		{"a root README naming a package", "README.md", roleNamed},
		{"a guide naming a package", "docs/guide.md", roleNamed},
		{"a guide naming nothing", "docs/nothing.md", roleNone},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			doc := piece(testCase.file, wholeFilePrefix, entities.Span{})

			got := classify(snap, []entities.Decl{doc, code, header})[doc.Key].role
			if got != testCase.want {
				t.Errorf("classify(%s) = %v, want %v", testCase.file, got, testCase.want)
			}
		})
	}
}
