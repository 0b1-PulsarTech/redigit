package plancache

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoToolchainHashesTheFilesGoEnvOnlyNames(t *testing.T) {
	repo, commit := newRepo(t, map[string]string{"go.mod": "module x\n"})

	testCases := []struct {
		name     string
		isDriver bool
		change   string
		wantSame bool
	}{
		{"unset", false, "", true},
		{"driver bytes changed", true, "driver", false},
		{"driver bytes the same", true, "", true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fake := newFakeGo(t, `{"GOROOT":"$ROOT","GOWORK":"off"}`)

			driver := writeFile(t, fake.root, "driver", "#!/bin/sh\necho one\n", 0o700)
			if testCase.isDriver {
				t.Setenv("GOPACKAGESDRIVER", driver)
			} else {
				t.Setenv("GOPACKAGESDRIVER", "off")
			}

			first, err := GoToolchain(t.Context(), repo, commit)
			if err != nil {
				t.Fatalf("GoToolchain() err = %v", err)
			}

			if testCase.change != "" {
				writeFile(t, fake.root, testCase.change, "#!/bin/sh\necho two\n", 0o700)
			}

			second, err := GoToolchain(t.Context(), repo, commit)
			if err != nil {
				t.Fatalf("GoToolchain() err = %v", err)
			}

			if (first == second) != testCase.wantSame {
				t.Fatalf("same key = %v, want %v", first == second, testCase.wantSame)
			}
		})
	}
}

func TestGoToolchainHashesAnImplicitDriver(t *testing.T) {
	repo, commit := newRepo(t, map[string]string{"go.mod": "module x\n"})

	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("find git: %v", err)
	}

	key := func(t *testing.T, fake fakeGo, pathDir, driverBody string) string {
		t.Helper()

		t.Setenv("PATH", strings.Join(
			[]string{fake.bin, pathDir, filepath.Dir(gitPath)},
			string(os.PathListSeparator),
		))
		t.Setenv("GOPACKAGESDRIVER", "")

		if driverBody != "" {
			writeFile(t, pathDir, "gopackagesdriver", driverBody, 0o700)
		}

		got, keyErr := GoToolchain(t.Context(), repo, commit)
		if keyErr != nil {
			t.Fatalf("GoToolchain() err = %v", keyErr)
		}

		return got
	}

	newFake := func(t *testing.T) fakeGo {
		t.Helper()

		fake := newFakeGo(t, `{"GOROOT":"$ROOT","GOWORK":"off"}`)
		writeFile(t, fake.bin, "go", "#!/bin/sh\ncase \"$1\" in\nenv) echo '{\"GOROOT\":\""+
			fake.root+"\",\"GOWORK\":\"off\"}' ;;\nversion) echo go-fake ;;\nesac\n", 0o700)

		return fake
	}

	t.Run("a driver on PATH is hashed", func(t *testing.T) {
		fake, dir := newFake(t), t.TempDir()

		one := key(t, fake, dir, "#!/bin/sh\necho one\n")
		same := key(t, fake, dir, "#!/bin/sh\necho one\n")
		two := key(t, fake, dir, "#!/bin/sh\necho two\n")

		if one != same || one == two {
			t.Fatalf("keys one=%s same=%s two=%s: the driver bytes do not key it", one, same, two)
		}
	})

	t.Run("no driver on PATH leaves the key plain", func(t *testing.T) {
		fake, dir := newFake(t), t.TempDir()

		first := key(t, fake, dir, "")
		if second := key(t, fake, dir, ""); first != second {
			t.Fatalf("keys differ with no driver: %s %s", first, second)
		}

		if withDriver := key(t, fake, dir, "#!/bin/sh\n"); withDriver == first {
			t.Fatal("a driver on PATH left the key unchanged")
		}
	})
}

func TestGoToolchainRefusesWhatItCannotHash(t *testing.T) {
	repo, commit := newRepo(t, map[string]string{"go.mod": "module x\n"})

	testCases := []struct {
		name        string
		environment string
		driver      string
		wantErr     string
	}{
		{"driver missing", `{"GOROOT":"$ROOT"}`, "/nonexistent/driver", "GOPACKAGESDRIVER"},
		{"driver relative with dot", `{"GOROOT":"$ROOT"}`, "./drv", "GOPACKAGESDRIVER"},
		{"driver relative in a subdir", `{"GOROOT":"$ROOT"}`, "bin/drv", "GOPACKAGESDRIVER"},
		{"a user's go.work", `{"GOROOT":"$ROOT","GOWORK":"/some/go.work"}`, "off", "GOWORK"},
		{"modfile with equals", `{"GOROOT":"$ROOT","GOFLAGS":"-modfile=x"}`, "off", "modfile"},
		{"modfile with two dashes", `{"GOROOT":"$ROOT","GOFLAGS":"--modfile=x"}`, "off", "modfile"},
		{
			"overlay with a separate value",
			`{"GOROOT":"$ROOT","GOFLAGS":"-mod=mod -overlay x"}`,
			"off",
			"overlay",
		},
		{
			"modfile in double quotes",
			`{"GOROOT":"$ROOT","GOFLAGS":"\"-modfile=/x/a.mod\""}`,
			"off",
			"modfile",
		},
		{
			"overlay in single quotes",
			`{"GOROOT":"$ROOT","GOFLAGS":"'-overlay=o.json'"}`,
			"off",
			"overlay",
		},
		{"an unrelated flag", `{"GOROOT":"$ROOT","GOFLAGS":"-mod=mod"}`, "off", ""},
		{"an undecodable GOFLAGS", `{"GOROOT":"$ROOT","GOFLAGS":5}`, "off", "GOFLAGS"},
		{"an undecodable GOWORK", `{"GOROOT":"$ROOT","GOWORK":["a"]}`, "off", "GOWORK"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			newFakeGo(t, testCase.environment)
			t.Setenv("GOPACKAGESDRIVER", testCase.driver)

			cwd := t.TempDir()
			if err := os.Mkdir(filepath.Join(cwd, "bin"), 0o700); err != nil {
				t.Fatalf("mkdir: %v", err)
			}

			writeFile(t, cwd, "drv", "#!/bin/sh\n", 0o700)
			writeFile(t, filepath.Join(cwd, "bin"), "drv", "#!/bin/sh\n", 0o700)
			t.Chdir(cwd)

			_, err := GoToolchain(t.Context(), repo, commit)

			if testCase.wantErr == "" {
				if err != nil {
					t.Fatalf("GoToolchain() err = %v, want none", err)
				}

				return
			}

			if !errors.Is(err, ErrToolchain) || !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf(
					"GoToolchain() err = %v, want ErrToolchain naming %s",
					err,
					testCase.wantErr,
				)
			}
		})
	}
}
