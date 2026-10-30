package format

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type formatCase struct {
	name      string
	mode      Mode
	timeout   time.Duration
	prepare   func(*testing.T, *Config)
	cancelled bool
	parallel  bool
	files     []File
	want      []string
	failed    []string
	cause     error
	noRun     bool
	spared    error
	settled   int
	exact     int
	atMost    int
}

func fakeConfig(mode Mode, timeout time.Duration) Config {
	return Config{
		BatchSize: 100,
		Workers:   2,
		Languages: []Language{{
			Name:       "go",
			Extensions: []string{".go"},
			SkipMarker: "Code generated",
			Steps: []Step{{
				Cmd:     []string{"fakefmt", "-w"},
				Mode:    mode,
				Timeout: Duration(timeout),
			}},
		}},
	}
}

func invocations(t *testing.T, log string) int {
	t.Helper()

	raw, err := os.ReadFile(log)
	if err != nil {
		return 0
	}

	return strings.Count(string(raw), "\n")
}

func numbered(count int) []File {
	files := make([]File, count)
	for index := range files {
		files[index] = File{Path: fmt.Sprintf("f%d.go", index), Content: []byte("x")}
	}

	return files
}

func pathsOf(files []File) []string {
	paths := make([]string, len(files))
	for index, file := range files {
		paths[index] = file.Path
	}

	return paths
}

func (testCase formatCase) run(t *testing.T) {
	t.Helper()

	log := testCase.logPath(t)

	config := fakeConfig(testCase.mode, testCase.timeout)
	if testCase.prepare != nil {
		testCase.prepare(t, &config)
	}

	engine, err := New(config)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := testCase.callerContext()
	defer cancel()

	got, failures := engine.FormatAll(ctx, testCase.files)

	want := testCase.want
	if want == nil {
		for _, file := range testCase.files {
			want = append(want, string(file.Content))
		}
	}

	for index, content := range want {
		if string(got[index].Content) != content || got[index].Path != testCase.files[index].Path {
			t.Errorf("file %d = %q, want %q", index, got[index].Content, content)
		}
	}

	testCase.checkFailures(t, failures)

	testCase.checkCount(t, invocations(t, log))
}

func (testCase formatCase) checkFailures(t *testing.T, failures []*Failure) {
	t.Helper()

	if len(failures) != len(testCase.failed) {
		t.Fatalf("failures = %v, want paths %v", failures, testCase.failed)
	}

	for index, failure := range failures {
		if failure.Path != testCase.failed[index] || failure.Tool != "fakefmt" {
			t.Errorf("failure = %+v", failure)
		}

		if testCase.cause != nil && index >= testCase.settled &&
			!errors.Is(failure, testCase.cause) {
			t.Errorf("failure %v does not wrap %v", failure, testCase.cause)
		}

		if testCase.spared != nil && errors.Is(failure, testCase.spared) {
			t.Errorf("failure %d wraps %v: %v", index, testCase.spared, failure)
		}
	}
}

func TestNew(t *testing.T) {
	testCases := []struct {
		name    string
		mutate  func(*Config)
		wantErr error
		missing bool
	}{
		{name: "a valid config", mutate: func(*Config) {}},
		{
			name: "a missing binary",
			mutate: func(config *Config) {
				config.Languages[0].Steps[0].Cmd = []string{"no-such-formatter-xyz"}
			},
			missing: true,
		},
		{
			name:    "an empty cmd",
			mutate:  func(config *Config) { config.Languages[0].Steps[0].Cmd = nil },
			wantErr: ErrConfig,
		},
		{
			name:    "an unknown mode",
			mutate:  func(config *Config) { config.Languages[0].Steps[0].Mode = "pipe" },
			wantErr: ErrConfig,
		},
		{
			name:    "an extension without a dot",
			mutate:  func(config *Config) { config.Languages[0].Extensions = []string{"go"} },
			wantErr: ErrConfig,
		},
		{
			name:    "an isolate pattern holding a slash",
			mutate:  func(config *Config) { config.Languages[0].Isolate = []string{"sub/*.cfg"} },
			wantErr: ErrConfig,
		},
		{
			name: "an extension claimed twice",
			mutate: func(config *Config) {
				config.Languages = append(config.Languages, config.Languages[0])
			},
			wantErr: ErrConfig,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			config := fakeConfig(ModeFiles, 0)
			testCase.mutate(&config)

			_, err := New(config)

			var missing *MissingToolError

			switch {
			case testCase.missing:
				if !errors.As(err, &missing) || missing.Tool != "no-such-formatter-xyz" {
					t.Fatalf("error = %v, want a MissingToolError", err)
				}
			case testCase.wantErr != nil:
				if !errors.Is(err, testCase.wantErr) {
					t.Fatalf("error = %v, want %v", err, testCase.wantErr)
				}
			case err != nil:
				t.Fatal(err)
			}
		})
	}
}

func withMutation(mutate func(*Config)) func(*testing.T, *Config) {
	return func(_ *testing.T, config *Config) { mutate(config) }
}

func TestFormatAll(t *testing.T) {
	testCases := []formatCase{
		{
			name:  "a non-go path and a generated file pass through",
			mode:  ModeFiles,
			files: []File{{"README.md", []byte("md")}, {"gen.go", []byte("// Code generated\nx")}},
		},
		{
			name: "each file picks its own language",
			mode: ModeFiles,
			prepare: withMutation(func(config *Config) {
				config.Languages[0].Extensions = []string{".go", ".rs"}
			}),
			files: []File{
				{"a.go", []byte("ab")},
				{"b.txt", []byte("cd")},
				{"c.rs", []byte("ef")},
			},
			want:  []string{"AB", "cd", "EF"},
			exact: 1,
		},
		{
			name: "if_wider runs a step on the wide blobs only",
			mode: ModeFiles,
			prepare: withMutation(func(config *Config) {
				config.Languages[0].Steps[0].IfWider = 5
			}),
			files: []File{
				{"short.go", []byte("ab")},
				{"long.go", []byte(strings.Repeat("a", 6))},
			},
			want: []string{"ab", "AAAAAA"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, testCase.run)
	}
}

func installFakeAt(t *testing.T, root string) {
	t.Helper()

	built, err := exec.LookPath("fakefmt")
	if err != nil {
		t.Fatal(err)
	}

	binary, err := os.ReadFile(built)
	if err != nil {
		t.Fatal(err)
	}

	if err = os.Mkdir(filepath.Join(root, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}

	if err = os.WriteFile(filepath.Join(root, "bin", "fmt"), binary, 0o700); err != nil {
		t.Fatal(err)
	}
}

func realConfig(t *testing.T) Config {
	t.Helper()

	for _, name := range []string{"gofumpt", "goimports", "golines"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Skipf("%s is not on PATH", name)
		}
	}

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/x\n\ngo 1.26\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	timeout := Duration(time.Minute)

	return Config{
		ConfigRoot: root,
		CopyFiles:  []string{"go.mod"},
		Languages: []Language{{
			Name:       "go",
			Extensions: []string{".go"},
			SkipMarker: "Code generated",
			Steps: []Step{
				{Cmd: []string{"gofumpt", "-w"}, Mode: ModeFiles, Timeout: timeout},
				{
					Cmd:     []string{"goimports", "-w", "-local", "example.com"},
					Mode:    ModeFiles,
					Timeout: timeout,
				},
				{Cmd: []string{"golines", "-w", "-m", "100"}, Mode: ModeFiles, Timeout: timeout},
			},
		}},
	}
}

func TestRealToolsFormat(t *testing.T) {
	const grouped = "package a\n\nimport (\n\t\"fmt\"\n\n\t\"github.com/x/y\"\n\n" +
		"\t\"example.com/x/z\"\n)\n\nvar (\n\t_ = fmt.Sprint\n\t_ = y.A\n\t_ = z.B\n)\n"

	testCases := []struct {
		name    string
		path    string
		input   string
		want    string
		wantErr bool
	}{
		{
			"an unformatted file",
			"a.go",
			"package a\nfunc  F( ) {\n}\n",
			"package a\n\nfunc F() {\n}\n",
			false,
		},
		{
			"an already formatted file",
			"a.go",
			"package a\n\nfunc F() {\n}\n",
			"package a\n\nfunc F() {\n}\n",
			false,
		},
		{"a syntax error", "a.go", "package a\nfunc (", "package a\nfunc (", true},
		{"a non go path", "a.txt", "func  F( ) {", "func  F( ) {", false},
		{
			"imports grouped with the local prefix last",
			"a.go",
			"package a\n\nimport (\n\t\"example.com/x/z\"\n\t\"github.com/x/y\"\n\t\"fmt\"\n)\n\n" +
				"var _ = fmt.Sprint\nvar _ = y.A\nvar _ = z.B\n",
			grouped,
			false,
		},
		{
			"a build tag header is preserved", "a.go",
			"//go:build linux\n\npackage a\nfunc F( ) {}\n",
			"//go:build linux\n\npackage a\n\nfunc F() {}\n", false,
		},
		{
			"a long line wrapped by golines",
			"a.go",
			"package a\n\nfunc F(first string, second string, third string, fourth string, " +
				"fifth string, sixth string, seventh string) {\n}\n",
			"package a\n\nfunc F(\n\tfirst string,\n\tsecond string,\n\tthird string,\n" +
				"\tfourth string,\n\tfifth string,\n\tsixth string,\n\tseventh string,\n) {\n}\n",
			false,
		},
	}

	engine, err := New(realConfig(t))
	if err != nil {
		t.Fatal(err)
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, failure := formatOne(engine, testCase.path, []byte(testCase.input))
			if (failure != nil) != testCase.wantErr {
				t.Fatalf("failure = %v, want failure %v", failure, testCase.wantErr)
			}

			if string(got) != testCase.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, testCase.want)
			}

			again, failure := formatOne(engine, testCase.path, got)
			if failure != nil && !testCase.wantErr {
				t.Fatal(failure)
			}

			if string(again) != string(got) {
				t.Errorf("not idempotent:\n%s\nthen:\n%s", got, again)
			}
		})
	}
}

func formatOne(engine *Engine, name string, content []byte) ([]byte, *Failure) {
	out, failures := engine.FormatAll(context.Background(), []File{{Path: name, Content: content}})
	if len(failures) > 0 {
		return content, failures[0]
	}

	return out[0].Content, nil
}

func (testCase formatCase) logPath(t *testing.T) string {
	t.Helper()

	if testCase.parallel {
		t.Parallel()

		return ""
	}

	log := filepath.Join(t.TempDir(), "invocations")
	t.Setenv("FAKEFMT_LOG", log)

	return log
}

func (testCase formatCase) checkCount(t *testing.T, count int) {
	t.Helper()

	if testCase.exact > 0 && count != testCase.exact {
		t.Errorf("invocations = %d, want %d", count, testCase.exact)
	}

	if testCase.noRun && count != 0 {
		t.Errorf("invocations = %d, want none", count)
	}

	if testCase.atMost > 0 && count > testCase.atMost {
		t.Errorf("invocations = %d, want at most %d", count, testCase.atMost)
	}
}

func (testCase formatCase) callerContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	if testCase.cancelled {
		cancel()
	}

	return ctx, cancel
}
