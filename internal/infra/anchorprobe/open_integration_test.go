//go:build integration

package anchorprobe

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang"
)

func gitIn(t testing.TB, dir string, args ...string) {
	t.Helper()

	out, err := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...).
		CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func openOn(t *testing.T, files map[string]string, verify string) *Probe {
	t.Helper()

	dir := t.TempDir()
	gitIn(t, dir, "init", "-q", "-b", "main")
	gitIn(t, dir, "config", "user.email", "tester@example.com")
	gitIn(t, dir, "config", "user.name", "Tester")

	for name, text := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}

		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-q", "-m", "feat: add")

	probe, err := Open(t.Context(), Settings{
		Repo: dir, Rev: "HEAD", CacheDir: t.TempDir(), Renderer: golang.NewSyntax(),
		Verify: json.RawMessage(verify),
	})
	if err != nil {
		t.Fatalf("Open() = %v", err)
	}

	t.Cleanup(func() { _ = probe.Close(t.Context()) })

	return probe
}

func TestOpenBuildsARealTreeWithEditsSpliced(t *testing.T) {
	t.Parallel()

	const (
		modFile = "module example.com/p\n\ngo 1.21\n"
		header  = "package p\n\nimport \"strconv\"\n\n"
		body    = "func A() int { n, _ := strconv.Atoi(\"1\"); return n }"
	)

	probe := openOn(t, map[string]string{"go.mod": modFile, "a.go": header + body + "\n"}, "")
	span := entities.Span{Start: len(header), End: len(header) + len(body)}
	keep := []entities.DeclKey{{File: "a.go", Kind: entities.KindFunc, Name: "A"}}
	atoi := "func A() int { n, _ := strconv.Atoi(\"0\"); return n }"
	testCases := []struct {
		name string
		text string
		want bool
	}{
		{name: "a pre-image that compiles", text: atoi, want: true},
		{name: "a pre-image that does not", text: "func A() int { return missing }"},
		{
			name: "a pre-image that no longer uses an import",
			text: "func A() int { return 1 }", want: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			verdict, err := probe.Builds(t.Context(), []anchoring.Edit{
				{File: "a.go", Span: span, Text: testCase.text, Keep: keep},
			})
			if err != nil || verdict.IsBuilt != testCase.want {
				t.Fatalf("Builds() = %+v, %v, want built = %v", verdict, err, testCase.want)
			}

			if testCase.want && strings.Contains(verdict.Detail, "imported") {
				t.Fatalf("detail = %q", verdict.Detail)
			}
		})
	}
}

func TestOpenGivesNoVerdictOnATreeThatAlreadyFails(t *testing.T) {
	t.Parallel()

	const (
		modFile = "module example.com/p\n\ngo 1.21\n"
		source  = "package p\n\nfunc A() int { return 1 }\n"
		broken  = "package p\n\nimport \"fmt\"\n\nfunc L() string { return fmt.Sprintf(\"%d\", \"x\") }\n"
	)

	span := entities.Span{Start: len("package p\n\n"), End: len(source) - 1}
	keep := []entities.DeclKey{{File: "a.go", Kind: entities.KindFunc, Name: "A"}}
	testCases := []struct {
		name  string
		files map[string]string
		text  string
	}{
		{
			name:  "a tree whose other file already fails to vet",
			files: map[string]string{"go.mod": modFile, "a.go": source, "l.go": broken},
			text:  "func A() int { return 0 }",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			probe := openOn(t, testCase.files, "")

			verdict, err := probe.Builds(t.Context(), []anchoring.Edit{
				{File: "a.go", Span: span, Text: testCase.text, Keep: keep},
			})
			if err != nil || verdict.IsBuilt || !verdict.IsUnchecked {
				t.Fatalf("Builds() = %+v, %v, want an unchecked verdict", verdict, err)
			}
		})
	}
}

func TestOpenHonoursTheVerifySection(t *testing.T) {
	t.Parallel()

	const (
		modFile  = "module example.com/p\n\ngo 1.21\n"
		workFile = "go 1.21\n\nuse (\n\t.\n\t./gone\n)\n"
		source   = "package p\n\nfunc A() int { return 1 }\n"
		redefine = `{"languages":{"go":{` +
			`"steps":[{"cmd":["env","GOWORK=off","go","build","./..."]}]}}}`
	)

	span := entities.Span{Start: len("package p\n\n"), End: len(source) - 1}
	testCases := []struct {
		name      string
		files     map[string]string
		verify    string
		file      string
		wantBuilt bool
		wantNote  string
	}{
		{
			name:  "a go.work naming a missing module leaves every anchor unchecked",
			files: map[string]string{"go.mod": modFile, "go.work": workFile, "a.go": source},
			file:  "a.go",
		},
		{
			name:      "the section's steps and env check the tree with GOWORK off",
			files:     map[string]string{"go.mod": modFile, "go.work": workFile, "a.go": source},
			verify:    redefine,
			file:      "a.go",
			wantBuilt: true,
		},
		{
			name: "an edit only in an excluded module reads unchecked and says so",
			files: map[string]string{
				"app/go.mod": modFile, "app/a.go": source,
				"lifted/go.mod": "module example.com/lifted\n\ngo 1.21\n", "lifted/a.go": source,
			},
			verify:   `{"exclude":["lifted"]}`,
			file:     "lifted/a.go",
			wantNote: "verify.exclude",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			probe := openOn(t, testCase.files, testCase.verify)

			verdict, err := probe.Builds(t.Context(), []anchoring.Edit{
				{
					File: testCase.file,
					Span: span,
					Text: "func A() int { return 0 }",
					Keep: []entities.DeclKey{
						{File: testCase.file, Kind: entities.KindFunc, Name: "A"},
					},
				},
			})
			if err != nil || verdict.IsBuilt != testCase.wantBuilt ||
				verdict.IsUnchecked == testCase.wantBuilt {
				t.Fatalf("Builds() = %+v, %v, want built = %v", verdict, err, testCase.wantBuilt)
			}

			if !strings.Contains(verdict.Detail, testCase.wantNote) {
				t.Fatalf("detail = %q, want %q", verdict.Detail, testCase.wantNote)
			}
		})
	}
}
