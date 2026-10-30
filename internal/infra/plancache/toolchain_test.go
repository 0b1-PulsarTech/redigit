package plancache

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

type fakeGo struct {
	root string
	bin  string
}

func (fake fakeGo) script(envCommand, marker string) string {
	return "#!/bin/sh\nROOT=\"" + fake.root + "\"\ncase \"$1\" in\nenv) " + envCommand +
		" ;;\nversion) cat \"" + filepath.Join(fake.root, "version") + "\" ;;\nesac\n# " + marker + "\n"
}

func (fake fakeGo) install(t *testing.T, envCommand, marker string) {
	t.Helper()

	writeFile(t, fake.bin, "go", fake.script(envCommand, marker), 0o700)
}

func newFakeGo(t *testing.T, environment string) fakeGo {
	t.Helper()

	root := t.TempDir()
	bin := filepath.Join(root, "bin")

	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	fake := fakeGo{root: root, bin: bin}
	fake.install(t, "cat \""+filepath.Join(root, "env.json")+"\"", "")
	writeFile(t, root, "version", "go-fake\n", 0o600)
	fake.setEnv(t, environment)

	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	return fake
}

func (fake fakeGo) setEnv(t *testing.T, environment string) {
	t.Helper()

	environment = strings.ReplaceAll(environment, "$ROOT", fake.root)
	writeFile(t, fake.root, "env.json", environment, 0o600)
}

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()

	full := append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)
	command := exec.CommandContext(t.Context(), "git", full...)
	command.Dir = dir

	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}

	return strings.TrimSpace(string(out))
}

func commitFiles(t *testing.T, repo string, files map[string]string) string {
	t.Helper()

	for name, content := range files {
		if content == "" {
			_ = os.Remove(filepath.Join(repo, name))

			continue
		}

		if err := os.MkdirAll(filepath.Dir(filepath.Join(repo, name)), 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		writeFile(t, repo, name, content, 0o600)
	}

	gitRun(t, repo, "add", "-A")
	gitRun(t, repo, "commit", "-q", "--allow-empty", "-m", "c")

	return gitRun(t, repo, "rev-parse", "HEAD")
}

func newRepo(t *testing.T, files map[string]string) (repo, commit string) {
	t.Helper()

	repo = t.TempDir()
	gitRun(t, repo, "init", "-q")

	return repo, commitFiles(t, repo, files)
}

func TestGoToolchainFollowsTheGoEnvironment(t *testing.T) {
	const environment = `{"GOROOT":"$ROOT","GOOS":"linux","GOMODCACHE":"/mod"}`

	fake := newFakeGo(t, environment)
	repo, commit := newRepo(t, map[string]string{"go.mod": "module x\n"})
	catEnv := "cat \"" + filepath.Join(fake.root, "env.json") + "\""

	digest := func() string {
		t.Helper()

		got, err := GoToolchain(t.Context(), repo, commit)
		if err != nil {
			t.Fatalf("GoToolchain() err = %v", err)
		}

		return got
	}

	base := digest()

	if digest() != base {
		t.Fatal("the digest of an unchanged toolchain changed")
	}

	testCases := []struct {
		name   string
		change func(*testing.T)
	}{
		{"a GOENV file setting", func(t *testing.T) {
			fake.setEnv(t, `{"GOROOT":"$ROOT","GOOS":"windows","GOMODCACHE":"/mod"}`)
		}},
		{"another module cache", func(t *testing.T) {
			fake.setEnv(t, `{"GOROOT":"$ROOT","GOOS":"linux","GOMODCACHE":"/nonexistent"}`)
		}},
		{"only the toolchain binary", func(t *testing.T) {
			fake.install(t, catEnv, "switched")
		}},
		{"only the version it prints", func(t *testing.T) {
			writeFile(t, fake.root, "version", "go-other\n", 0o600)
		}},
		{"GOPACKAGESDRIVER", func(t *testing.T) {
			t.Setenv("GOPACKAGESDRIVER", writeFile(t, fake.root, "driver", "x", 0o700))
		}},
		{"GODEBUG", func(t *testing.T) { t.Setenv("GODEBUG", "gotypesalias=0") }},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fake.setEnv(t, environment)
			fake.install(t, catEnv, "")
			writeFile(t, fake.root, "version", "go-fake\n", 0o600)

			testCase.change(t)

			if digest() == base {
				t.Fatalf("the digest ignored %s", testCase.name)
			}
		})
	}
}

func TestGoToolchainProbesTheCommitsOwnModule(t *testing.T) {
	const probeEnv = `printf '{"GOROOT":"%s","GOMOD":"%s/go.mod","PROBE":"%s"}' "$ROOT" "$PWD" ` +
		`"$(sed -n 's/^toolchain //p' go.mod 2>/dev/null)$(test -f go.work && echo +work)"`

	fake := newFakeGo(t, "")
	fake.install(t, probeEnv, "")

	repo, older := newRepo(t, map[string]string{"go.mod": "module x\n\ntoolchain go1.24.1\n"})
	newer := commitFiles(t, repo, map[string]string{"go.mod": "module x\n\ntoolchain go1.25.2\n"})
	withWork := commitFiles(t, repo, map[string]string{"go.work": "go 1.25\n"})
	bare := commitFiles(t, repo, map[string]string{"go.mod": "", "go.work": ""})

	keys := map[string]string{}

	for name, commit := range map[string]string{
		"older": older, "newer": newer, "work": withWork, "bare": bare,
	} {
		first, err := GoToolchain(t.Context(), repo, commit)
		if err != nil {
			t.Fatalf("%s: GoToolchain() err = %v", name, err)
		}

		if again, _ := GoToolchain(t.Context(), repo, commit); again != first {
			t.Errorf("%s: the key follows the throwaway probe dir", name)
		}

		for seen, key := range keys {
			if key == first {
				t.Errorf("%s and %s share a key: the probe missed a file", name, seen)
			}
		}

		keys[name] = first
	}
}

func TestGoToolchainFailures(t *testing.T) {
	repo, commit := newRepo(t, map[string]string{"go.mod": "module x\n"})

	testCases := []struct {
		name        string
		environment string
	}{
		{"not json", "not json"},
		{"no GOROOT", `{"GOOS":"linux"}`},
		{"no go binary in GOROOT", `{"GOROOT":"/nonexistent"}`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			newFakeGo(t, testCase.environment)

			if _, err := GoToolchain(t.Context(), repo, commit); !errors.Is(err, ErrToolchain) {
				t.Fatalf("GoToolchain() err = %v, want ErrToolchain", err)
			}
		})
	}

	t.Run("no go on PATH", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())

		if _, err := GoToolchain(t.Context(), repo, commit); !errors.Is(err, ErrToolchain) {
			t.Fatalf("GoToolchain() err = %v, want ErrToolchain", err)
		}
	})

	t.Run("a directory that is not a repository", func(t *testing.T) {
		newFakeGo(t, `{"GOROOT":"$ROOT"}`)

		if _, err := GoToolchain(t.Context(), t.TempDir(), commit); !errors.Is(err, ErrToolchain) {
			t.Fatalf("GoToolchain() err = %v, want ErrToolchain", err)
		}
	})
}

func TestGoToolchainIgnoresTheCompilerTempDir(t *testing.T) {
	const flags = `"GOGCCFLAGS":"-ffile-prefix-map=/tmp/go-build%s=/tmp/go-build"`

	repo, commit := newRepo(t, map[string]string{"go.mod": "module x\n"})

	testCases := []struct {
		name     string
		other    string
		wantSame bool
	}{
		{"another temp dir", `{"GOROOT":"$ROOT",` + flags + `}`, true},
		{"another compiler", `{"GOROOT":"$ROOT","CC":"clang",` + flags + `}`, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fake := newFakeGo(t, `{"GOROOT":"$ROOT",`+strings.Replace(flags, "%s", "111", 1)+`}`)

			first, err := GoToolchain(t.Context(), repo, commit)
			if err != nil {
				t.Fatalf("GoToolchain() err = %v", err)
			}

			fake.setEnv(t, strings.Replace(testCase.other, "%s", "222", 1))

			second, err := GoToolchain(t.Context(), repo, commit)
			if err != nil {
				t.Fatalf("GoToolchain() err = %v", err)
			}

			if (first == second) != testCase.wantSame {
				t.Fatalf("same digest = %v, want %v", first == second, testCase.wantSame)
			}
		})
	}
}

func TestGoToolchainKeepsAUsersGOWORKAndGOMOD(t *testing.T) {
	const probeEnv = `printf '{"GOROOT":"%s","GOWORK":"%s","GOMOD":"%s"}' "$ROOT" ` +
		`"${GOWORK-$PWD/go.work}" "${GOMOD-$PWD/go.mod}"`

	fake := newFakeGo(t, "")
	fake.install(t, probeEnv, "")

	repo, commit := newRepo(t, map[string]string{"go.mod": "module x\n", "go.work": "go 1.25\n"})

	key := func(t *testing.T, name, value string, isSet bool) string {
		t.Helper()

		t.Setenv(name, "")

		if !isSet {
			_ = os.Unsetenv(name)
		} else {
			t.Setenv(name, value)
		}

		got, err := GoToolchain(t.Context(), repo, commit)
		if err != nil {
			t.Fatalf("GoToolchain() err = %v", err)
		}

		return got
	}

	explicitWork := writeFile(t, t.TempDir(), "go.work", "go 1.25\n", 0o600)

	unset := key(t, "GOWORK", "", false)

	testCases := []struct {
		name     string
		variable string
		value    string
		isSet    bool
		wantSame bool
	}{
		{"unset again", "GOWORK", "", false, true},
		{"GOWORK=off", "GOWORK", "off", true, false},
		{"GOWORK empty", "GOWORK", "", true, false},
		{"an explicit GOMOD", "GOMOD", "/elsewhere/go.mod", true, false},
		{"the no-module GOMOD", "GOMOD", os.DevNull, true, true},
	}

	seen := map[string]string{}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := key(t, testCase.variable, testCase.value, testCase.isSet)

			if (got == unset) != testCase.wantSame {
				t.Fatalf("same as unset = %v, want %v", got == unset, testCase.wantSame)
			}

			if other, dup := seen[got]; dup && !testCase.wantSame {
				t.Fatalf("shares a key with %s", other)
			}

			seen[got] = testCase.name
		})
	}

	t.Run("an explicit GOWORK is refused", func(t *testing.T) {
		t.Setenv("GOWORK", explicitWork)

		if _, err := GoToolchain(t.Context(), repo, commit); !errors.Is(err, ErrToolchain) {
			t.Fatalf("GoToolchain() err = %v, want ErrToolchain", err)
		}
	})
}

func TestGoToolchainLeavesNoProbeDir(t *testing.T) {
	repo, commit := newRepo(t, map[string]string{"go.mod": "module x\n"})

	testCases := []struct {
		name    string
		goBody  string
		repo    string
		wantErr bool
	}{
		{"a success", "cat \"$ROOT/env.json\"", repo, false},
		{"a failing go", "exit 1", repo, true},
		{"a failing read", "cat \"$ROOT/env.json\"", t.TempDir(), true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fake := newFakeGo(t, `{"GOROOT":"$ROOT"}`)
			fake.install(t, testCase.goBody, "")

			scratch := t.TempDir()
			t.Setenv("TMPDIR", scratch)

			_, err := GoToolchain(t.Context(), testCase.repo, commit)
			if (err != nil) != testCase.wantErr {
				t.Fatalf("GoToolchain() err = %v, want error = %v", err, testCase.wantErr)
			}

			left, readErr := os.ReadDir(scratch)
			if readErr != nil || len(left) != 0 {
				t.Fatalf("temp dir holds %v (%v), want it empty", left, readErr)
			}
		})
	}
}

func TestGoToolchainWritesTheProbeFilesPrivately(t *testing.T) {
	fake := newFakeGo(t, "")
	fake.install(t, `stat -c %a go.mod go.work > "$ROOT/modes"; cat "$ROOT/env.json"`, "")
	fake.setEnv(t, `{"GOROOT":"$ROOT"}`)

	repo, commit := newRepo(t, map[string]string{"go.mod": "module x\n", "go.work": "go 1.25\n"})

	if _, err := GoToolchain(t.Context(), repo, commit); err != nil {
		t.Fatalf("GoToolchain() err = %v", err)
	}

	modes, err := os.ReadFile(filepath.Join(fake.root, "modes"))
	if err != nil || string(modes) != "600\n600\n" {
		t.Fatalf("probe file modes = %q (%v), want 600 for both", modes, err)
	}
}

func TestGoToolchainCarriesTheGoStderr(t *testing.T) {
	fake := newFakeGo(t, "")
	fake.install(t, "echo 'toolchain download failed' >&2; exit 1", "")

	repo, commit := newRepo(t, map[string]string{"go.mod": "module x\n"})

	_, err := GoToolchain(t.Context(), repo, commit)
	if !errors.Is(err, ErrToolchain) ||
		!strings.Contains(err.Error(), "toolchain download failed") {
		t.Fatalf("GoToolchain() err = %v, want ErrToolchain with the stderr text", err)
	}
}

func TestExcerpt(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		stderr  string
		wantLen int
	}{
		{"empty", "", 0},
		{"trimmed", "  boom\n", 4},
		{"capped ascii", strings.Repeat("a", 500), 240},
		{"capped on a rune start", "a" + strings.Repeat("\u00e9", 200), 239},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := excerpt([]byte(testCase.stderr))
			if len(got) != testCase.wantLen || !utf8.ValidString(got) {
				t.Fatalf("excerpt = %d bytes, valid = %v, want %d bytes of valid utf8",
					len(got), utf8.ValidString(got), testCase.wantLen)
			}
		})
	}
}
