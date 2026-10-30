package checkrun

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
)

const unsetGoWork = "GOWORK_SET=false\n"

func unitInWorkspace(text string) verifying.Unit {
	unit := goUnit()
	unit.Workspace = text

	return unit
}

func dumpOf(t *testing.T, unit verifying.Unit) string {
	t.Helper()

	dump := filepath.Join(t.TempDir(), "env")
	runner, slotDir := runnerFor(t, fake("env", dump))

	outcome, err := runner.Check(context.Background(), slotDir, unit)
	if err != nil || outcome.IsFailed {
		t.Fatalf("check: outcome = %+v err = %v", outcome, err)
	}

	content, err := os.ReadFile(dump)
	if err != nil {
		t.Fatalf("read dump: %v", err)
	}

	return string(content)
}

func TestCheckGoWorkMode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		unit verifying.Unit
		want string
	}{
		{"a tree without go.work builds with workspaces off", goUnit(), "GOWORK=off\n"},
		{
			"a tree with go.work leaves the go command to find it",
			unitInWorkspace(".\x00use ./mod"), unsetGoWork,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := dumpOf(t, testCase.unit); !strings.Contains(got, testCase.want) {
				t.Fatalf("dump = %q, want it to hold %q", got, testCase.want)
			}
		})
	}
}

func TestCheckDropsAnInheritedGoWork(t *testing.T) {
	t.Setenv("GOWORK", filepath.Join(t.TempDir(), "elsewhere", "go.work"))

	testCases := []struct {
		name string
		unit verifying.Unit
		want string
	}{
		{"a workspace tree never sees the caller's GOWORK", unitInWorkspace("w"), unsetGoWork},
		{"a tree without go.work overrides it with off", goUnit(), "GOWORK=off\n"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := dumpOf(t, testCase.unit); !strings.Contains(got, testCase.want) {
				t.Fatalf("dump = %q, want it to hold %q", got, testCase.want)
			}
		})
	}
}

func TestCheckPinsTheGoEnvironment(t *testing.T) {
	t.Setenv("GO111MODULE", "off")
	t.Setenv("GOFLAGS", "-mod=mod")
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "elsewhere"))

	got := dumpOf(t, goUnit())

	for _, want := range []string{"GO111MODULE=on\n", "GOFLAGS=-mod=mod -buildvcs=false\n"} {
		if !strings.Contains(got, want) {
			t.Fatalf("dump = %q, want it to hold %q", got, want)
		}
	}

	if strings.Contains(got, "GIT_DIR=") {
		t.Fatalf("dump = %q, the caller's GIT_DIR leaked into the step", got)
	}
}

func TestCheckKeepsGoFlagsStoredInTheGoEnvFile(t *testing.T) {
	envFile := filepath.Join(t.TempDir(), "goenv")
	if err := os.WriteFile(envFile, []byte("GOFLAGS=-tags=feat\n"), 0o600); err != nil {
		t.Fatalf("write goenv: %v", err)
	}

	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOENV", envFile)

	testCases := []struct {
		name  string
		flags string
		want  string
	}{
		{"an unset variable defers to the file", "", "GOFLAGS=-tags=feat -buildvcs=false\n"},
		{"a set variable wins over the file", "-mod=mod", "GOFLAGS=-mod=mod -buildvcs=false\n"},
		{"a blank variable still wins over the file", " ", "GOFLAGS=-buildvcs=false\n"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("GOFLAGS", testCase.flags)

			if got := dumpOf(t, goUnit()); !strings.Contains(got, testCase.want) {
				t.Fatalf("dump = %q, want it to hold %q", got, testCase.want)
			}
		})
	}
}

func TestIdentityFollowsGoFlagsStoredInTheGoEnvFile(t *testing.T) {
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOFLAGS", "")

	base := Config{Languages: map[string]Language{"go": {Steps: []Step{fake("pass")}}}}
	identityWith := func(content string) string {
		envFile := filepath.Join(t.TempDir(), "goenv")
		if err := os.WriteFile(envFile, []byte(content), 0o600); err != nil {
			t.Fatalf("write goenv: %v", err)
		}

		t.Setenv("GOENV", envFile)

		return identityOf(t, base, goUnit())
	}

	first, same, other := identityWith("GOFLAGS=-tags=a\n"),
		identityWith("GOFLAGS=-tags=a\n"), identityWith("GOFLAGS=-tags=b\n")
	if first != same || first == other {
		t.Fatalf("identities = %q, %q, %q, want the file's GOFLAGS to decide", first, same, other)
	}
}

func TestStepEnvScrubsGitVariables(t *testing.T) {
	t.Setenv("GIT_DIR", "/elsewhere")
	t.Setenv("GIT_INDEX_FILE", "/elsewhere/index")
	t.Setenv("REDIGIT_KEEP", "yes")

	env, _ := stepEnv(rustUnit(), nil, "")

	for _, entry := range env {
		if strings.HasPrefix(entry, "GIT_") {
			t.Fatalf("env still holds %q", entry)
		}
	}

	if !slices.Contains(env, "REDIGIT_KEEP=yes") {
		t.Fatal("scrubbing git variables dropped an unrelated one")
	}
}
