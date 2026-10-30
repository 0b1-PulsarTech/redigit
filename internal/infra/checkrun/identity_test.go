package checkrun

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
)

func rustUnit() verifying.Unit {
	return verifying.Unit{Module: verifying.Module{Dir: "mod", Path: "ex/mod", Language: "rust"}}
}

func identityOf(t *testing.T, cfg Config, unit verifying.Unit) string {
	t.Helper()

	got, err := mustRunner(t, cfg).Identity(context.Background(), unit)
	if err != nil {
		t.Fatalf("identity: %v", err)
	}

	return got
}

func TestIdentity(t *testing.T) {
	t.Parallel()

	base := Config{Languages: map[string]Language{"go": {Steps: []Step{fake("pass")}}}}
	withVersion := Config{Languages: map[string]Language{
		"go": {Steps: []Step{fake("pass")}, Version: []string{"fakecheck", "version"}},
	}}
	baseID := identityOf(t, base, goUnit())

	testCases := []struct {
		name     string
		cfg      Config
		unit     verifying.Unit
		wantSame bool
	}{
		{"same input", base, goUnit(), true},
		{
			"tags change it through the default go steps",
			Config{Languages: map[string]Language{"go": GoDefault([]string{"integration"})}},
			goUnit(), false,
		},
		{
			"another step argv changes it",
			Config{Languages: map[string]Language{"go": {Steps: []Step{fake("pass", "extra")}}}},
			goUnit(), false,
		},
		{
			"a tolerated text changes it",
			Config{Languages: map[string]Language{
				"go": {Steps: []Step{
					{Argv: []string{"fakecheck", "pass"}, Tolerate: []string{"x"}},
				}},
			}},
			goUnit(), false,
		},
		{"a version probe changes it", withVersion, goUnit(), false},
		{"a workspace changes it", base, unitInWorkspace("W"), false},
		{"another workspace changes it too", base, unitInWorkspace("V"), false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := identityOf(t, testCase.cfg, testCase.unit)
			if (got == baseID) != testCase.wantSame {
				t.Fatalf("same = %v, want %v", got == baseID, testCase.wantSame)
			}
		})
	}
}

func TestIdentityFollowsTheBuildEnvironment(t *testing.T) {
	base := Config{Languages: map[string]Language{"go": {Steps: []Step{fake("pass")}}}}
	baseID := identityOf(t, base, goUnit())

	testCases := []struct {
		name  string
		key   string
		value string
	}{
		{"GOFLAGS", "GOFLAGS", "-mod=mod"},
		{"CGO_ENABLED", "CGO_ENABLED", "0"},
		{"GOOS", "GOOS", "plan9"},
		{"GOARCH", "GOARCH", "riscv64"},
		{"GOTOOLCHAIN", "GOTOOLCHAIN", "local"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv(testCase.key, testCase.value)

			if got := identityOf(t, base, goUnit()); got == baseID {
				t.Fatalf("identity ignores %s=%s", testCase.key, testCase.value)
			}
		})
	}
}

func TestIdentityFollowsTheCgoAndToolEnvironment(t *testing.T) {
	useFakeGo(t)

	base := Config{Languages: map[string]Language{"go": {Steps: []Step{fake("pass")}}}}

	testCases := []struct {
		name string
		key  string
	}{
		{"cgo cflags allow", "CGO_CFLAGS_ALLOW"},
		{"cgo cflags disallow", "CGO_CFLAGS_DISALLOW"},
		{"cgo cppflags allow", "CGO_CPPFLAGS_ALLOW"},
		{"cgo cxxflags disallow", "CGO_CXXFLAGS_DISALLOW"},
		{"cgo ldflags allow", "CGO_LDFLAGS_ALLOW"},
		{"cgo ldflags disallow", "CGO_LDFLAGS_DISALLOW"},
		{"pkg-config binary", "PKG_CONFIG"},
		{"pkg-config path", "PKG_CONFIG_PATH"},
		{"pkg-config sysroot", "PKG_CONFIG_SYSROOT_DIR"},
		{"archiver", "AR"},
		{"goroot", "GOROOT"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv(testCase.key, "")

			if err := os.Unsetenv(testCase.key); err != nil {
				t.Fatalf("unset: %v", err)
			}

			unset := identityOf(t, base, goUnit())

			t.Setenv(testCase.key, "-fgnu-tm")

			set := identityOf(t, base, goUnit())

			t.Setenv(testCase.key, "")

			empty := identityOf(t, base, goUnit())

			if unset == set || unset == empty || set == empty {
				t.Fatalf("identity ignores %s: unset, set and empty must all differ", testCase.key)
			}
		})
	}
}

func TestIdentityFollowsDeclaredEnvNames(t *testing.T) {
	cfg := Config{Languages: map[string]Language{
		"rust": {Steps: []Step{fake("pass")}, Env: []string{"REDIGIT_TEST_RUSTFLAGS"}},
	}}
	plain := Config{Languages: map[string]Language{"rust": {Steps: []Step{fake("pass")}}}}

	unset, plainUnset := identityOf(t, cfg, rustUnit()), identityOf(t, plain, rustUnit())

	t.Setenv("REDIGIT_TEST_RUSTFLAGS", "-Cdebuginfo=2")

	if got := identityOf(t, cfg, rustUnit()); got == unset {
		t.Fatal("a declared env name does not enter the identity")
	}

	if got := identityOf(t, plain, rustUnit()); got == plainUnset {
		t.Fatal("an unknown env name no longer enters the identity by default")
	}
}

func TestIdentityKeysADeclaredNameOfTheUnhashedClass(t *testing.T) {
	cfg := Config{Languages: map[string]Language{
		"rust": {Steps: []Step{fake("pass")}, Env: []string{"TMPDIR"}},
	}}
	plain := Config{Languages: map[string]Language{"rust": {Steps: []Step{fake("pass")}}}}

	t.Setenv("TMPDIR", "/tmp/before")

	declared, undeclared := identityOf(t, cfg, rustUnit()), identityOf(t, plain, rustUnit())

	t.Setenv("TMPDIR", "/tmp/after")

	if got := identityOf(t, cfg, rustUnit()); got == declared {
		t.Fatal("a declared unhashed-class name does not enter the identity")
	}

	if got := identityOf(t, plain, rustUnit()); got != undeclared {
		t.Fatal("an undeclared unhashed-class name moved the identity")
	}
}

func TestIdentityErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		cfg     Config
		unit    verifying.Unit
		wantErr error
	}{
		{
			"a failing version probe is a builder error",
			Config{Languages: map[string]Language{
				"go": {
					Steps:   []Step{fake("pass")},
					Version: []string{"fakecheck", "brokenversion"},
				},
			}},
			goUnit(), ErrVersion,
		},
		{
			"a version tool that is not installed",
			Config{Languages: map[string]Language{
				"go": {
					Steps:   []Step{fake("pass")},
					Version: []string{"redigit-no-such-tool"},
				},
			}},
			goUnit(), ErrToolMissing,
		},
		{
			"an unconfigured language",
			Config{Languages: map[string]Language{"go": {Steps: []Step{fake("pass")}}}},
			verifying.Unit{Module: verifying.Module{Dir: "x", Language: "cobol"}},
			ErrNoSteps,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := mustRunner(t, testCase.cfg).Identity(context.Background(), testCase.unit)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestIdentityErrorCarriesTheBoundedProbeStderr(t *testing.T) {
	testCases := []struct {
		name        string
		wantPresent string
		wantAbsent  string
	}{
		{"the last line", "cache program exploded", ""},
		{"not the first line", "", "noise 0\n"},
	}

	cfg := Config{Languages: map[string]Language{
		"go": {Steps: []Step{fake("pass")}, Version: []string{"fakecheck", "brokenversion"}},
	}}

	_, err := mustRunner(t, cfg).Identity(context.Background(), goUnit())
	if !errors.Is(err, ErrVersion) {
		t.Fatalf("err = %v, want ErrVersion", err)
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			message := err.Error()
			if testCase.wantPresent != "" && !strings.Contains(message, testCase.wantPresent) {
				t.Fatalf("error %q lacks %q", message, testCase.wantPresent)
			}

			if testCase.wantAbsent != "" && strings.Contains(message, testCase.wantAbsent) {
				t.Fatalf("error holds %q", testCase.wantAbsent)
			}
		})
	}
}

func TestIdentityProbesTheVersionOnce(t *testing.T) {
	t.Parallel()

	log := filepath.Join(t.TempDir(), "log")
	runner := mustRunner(t, Config{Languages: map[string]Language{
		"go": {Steps: []Step{fake("pass")}, Version: []string{"fakecheck", "version", log}},
	}})

	for range 3 {
		if _, err := runner.Identity(context.Background(), goUnit()); err != nil {
			t.Fatalf("identity: %v", err)
		}
	}

	content, _ := os.ReadFile(log)
	if lines := strings.Count(string(content), "\n"); lines != 1 {
		t.Fatalf("version probed %d times, want once", lines)
	}
}

func TestIdentityProbesInANeutralDirWithTheStepEnvironment(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		unit verifying.Unit
		want string
	}{
		{"a tree without go.work", goUnit(), "GOWORK=off\n"},
		{"a tree with go.work", unitInWorkspace("W"), unsetGoWork},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dump := filepath.Join(t.TempDir(), "env")
			runner := mustRunner(t, Config{Languages: map[string]Language{
				"go": {Steps: []Step{fake("pass")}, Version: []string{"fakecheck", "env", dump}},
			}})

			if _, err := runner.Identity(context.Background(), testCase.unit); err != nil {
				t.Fatalf("identity: %v", err)
			}

			content, err := os.ReadFile(dump)
			if err != nil {
				t.Fatalf("read dump: %v", err)
			}

			wantDir := "cwd=/\n"
			if !strings.Contains(string(content), testCase.want) ||
				!strings.Contains(string(content), wantDir) {
				t.Fatalf("probe saw %q, want %q in %q", content, testCase.want, wantDir)
			}
		})
	}
}

func useFakeGo(t *testing.T) {
	t.Helper()

	t.Setenv("PATH", os.Getenv(fakeGoDirEnv)+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestIdentityFollowsTheToolchainReport(t *testing.T) {
	useFakeGo(t)

	base := Config{Languages: map[string]Language{"go": {Steps: []Step{fake("pass")}}}}
	baseID := identityOf(t, base, goUnit())

	testCases := []struct {
		name string
		key  string
	}{
		{"GOVERSION", "GOVERSION"},
		{"GOWASM", "GOWASM"},
		{"GOMIPS64", "GOMIPS64"},
		{"GOFIPS140", "GOFIPS140"},
		{"GODEBUG", "GODEBUG"},
		{"GO111MODULE", "GO111MODULE"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("FAKEGO_"+testCase.key, "changed")

			if got := identityOf(t, base, goUnit()); got == baseID {
				t.Fatalf("identity ignores %s", testCase.key)
			}
		})
	}
}

func TestIdentityCarriesTheForcedGoEnvironment(t *testing.T) {
	useFakeGo(t)
	t.Setenv("GOFLAGS", "-mod=mod")
	t.Setenv("GO111MODULE", "off")

	base := Config{Languages: map[string]Language{"go": {Steps: []Step{fake("pass")}}}}
	got := identityOf(t, base, goUnit())

	for _, want := range []string{"-mod=mod -buildvcs=false", `"GO111MODULE":"on"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("identity = %q, want it to hold %q", got, want)
		}
	}
}

func TestIdentityProbesFromTheFilesystemRoot(t *testing.T) {
	useFakeGo(t)
	t.Setenv("GOENV", "off")

	testCases := []struct {
		name  string
		flags string
		want  []string
	}{
		{"an unset GOFLAGS also asks the env file", "", []string{"env", "env", "version"}},
		{"a set GOFLAGS needs no file probe", "-mod=mod", []string{"env", "version"}},
		{"a blank GOFLAGS counts as set", " ", []string{"env", "version"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			log := filepath.Join(t.TempDir(), "log")
			t.Setenv("FAKEGO_LOG", log)
			t.Setenv("GOFLAGS", testCase.flags)

			runner := mustRunner(t, Config{Languages: map[string]Language{
				"go": {Steps: []Step{fake("pass")}, Version: []string{"go", "version"}},
			}})

			if _, err := runner.Identity(context.Background(), goUnit()); err != nil {
				t.Fatalf("identity: %v", err)
			}

			content, err := os.ReadFile(log)
			if err != nil {
				t.Fatalf("read log: %v", err)
			}

			var probes []string

			for line := range strings.SplitSeq(strings.TrimSpace(string(content)), "\n") {
				probe, ok := strings.CutSuffix(line, " cwd=/")
				if !ok {
					t.Fatalf("probe %q did not run in /, log %q", line, content)
				}

				probes = append(probes, probe)
			}

			slices.Sort(probes)

			if !slices.Equal(probes, testCase.want) {
				t.Fatalf("probes = %v, want %v", probes, testCase.want)
			}
		})
	}
}

func TestIdentityRefusesAnAmbientModule(t *testing.T) {
	useFakeGo(t)

	testCases := []struct {
		name    string
		key     string
		value   string
		wantErr error
	}{
		{"a go.mod above the probe", "GOMOD", "/repo/go.mod", ErrAmbient},
		{"a go.work above the probe", "GOWORK", "/repo/go.work", ErrAmbient},
		{"no module at all", "GOMOD", os.DevNull, nil},
		{"an empty module report", "GOMOD", "", nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("FAKEGO_"+testCase.key, testCase.value)

			base := Config{Languages: map[string]Language{"go": {Steps: []Step{fake("pass")}}}}

			_, err := mustRunner(t, base).Identity(context.Background(), goUnit())
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}
