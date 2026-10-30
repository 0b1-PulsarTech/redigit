package session

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/infra/plancache"
)

func testSet(t *testing.T, args ...string) (*flag.FlagSet, bootstrap.Config) {
	t.Helper()

	cfg := bootstrap.Defaults()
	set := flag.NewFlagSet("test", flag.ContinueOnError)

	set.StringVar(&cfg.Repo, "repo", "", "")
	set.StringVar(&cfg.Rev, "rev", cfg.Rev, "")
	set.StringVar(&cfg.ServerCommand, "server", cfg.ServerCommand, "")
	set.StringVar(&cfg.Resolver, "resolver", cfg.Resolver, "")
	set.Func("virtual", "", func(value string) error {
		if cfg.Virtual == nil {
			cfg.Virtual = map[string]string{}
		}

		id, file, _ := strings.Cut(value, "=")
		cfg.Virtual[id] = file

		return nil
	})

	for _, name := range []string{
		"tags", "layers", "features", "chunk-map", "config", "blob-dir", "cpuprofile",
		"memprofile", "trace", "mem-limit", "mode",
	} {
		set.String(name, "", "")
	}

	set.Int("far", 0, "")
	set.Bool("timings", false, "")
	set.Bool("from-cache", false, "")

	if err := set.Parse(args); err != nil {
		t.Fatalf("parse(%v) err = %v", args, err)
	}

	return set, cfg
}

func TestClassifyFlag(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		want FlagClass
	}{
		{"tags", ClassKey},
		{"virtual", ClassKey},
		{"resolver", ClassKey},
		{"mode", ClassKey},
		{"layers", ClassFile},
		{"features", ClassFile},
		{"chunk-map", ClassFile},
		{"config", ClassFile},
		{"far", ClassRunOnly},
		{"blob-dir", ClassRunOnly},
		{"from-cache", ClassRunOnly},
		{"timings", ClassRunOnly},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, isKnown := Classify(testCase.name)
			if !isKnown || got != testCase.want {
				t.Fatalf(
					"Classify(%q) = %v, %v, want %v",
					testCase.name,
					got,
					isKnown,
					testCase.want,
				)
			}
		})
	}

	if got, isKnown := Classify("brand-new"); isKnown || got != ClassKey {
		t.Fatalf("Classify(unknown) = %v, %v, want an unknown key flag", got, isKnown)
	}
}

func TestCacheFlagsLeaveOutRunOnlyFlags(t *testing.T) {
	t.Parallel()

	layers := filepath.Join(t.TempDir(), "layers")
	if err := os.WriteFile(layers, []byte("1 libs\n"), 0o600); err != nil {
		t.Fatalf("write layers: %v", err)
	}

	set, _ := testSet(t,
		"--repo", "/src", "--tags", "a,b", "--far", "3", "--cpuprofile", "cpu", "--from-cache",
		"--timings", "--blob-dir", "/tmp/blobs", "--layers", layers,
	)

	want := []plancache.Flag{
		{Name: "layers", Value: layers, IsFile: true},
		{Name: "mode", Value: "history"},
		{Name: "repo", Value: "/src"},
		{Name: "resolver", Value: "typed"},
		{Name: "tags", Value: "a,b"},
	}
	if got := Flags(set, nil); !reflect.DeepEqual(got, want) {
		t.Fatalf("Flags() = %+v, want %+v", got, want)
	}
}

func TestCacheFlagsKeyTheEffectiveMode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		args []string
		want string
	}{
		{"the default is history", []string{"--repo", "/src"}, "history"},
		{"history spelled out", []string{"--repo", "/src", "--mode", "history"}, "history"},
		{"lineage", []string{"--repo", "/src", "--mode", "lineage"}, "lineage"},
	}

	keyed := map[string][]plancache.Flag{}

	for _, testCase := range testCases {
		set, _ := testSet(t, testCase.args...)
		got := Flags(set, nil)
		keyed[testCase.name] = got

		index := slices.IndexFunc(
			got,
			func(flag plancache.Flag) bool { return flag.Name == "mode" },
		)
		if index < 0 || got[index].Value != testCase.want {
			t.Fatalf("%s: Flags() = %+v, want mode %q", testCase.name, got, testCase.want)
		}
	}

	if !reflect.DeepEqual(keyed["the default is history"], keyed["history spelled out"]) {
		t.Fatalf("default %+v and explicit history %+v key differently",
			keyed["the default is history"], keyed["history spelled out"])
	}

	if reflect.DeepEqual(keyed["history spelled out"], keyed["lineage"]) {
		t.Fatalf("lineage and history key alike: %+v", keyed["lineage"])
	}
}

func TestCacheFlagsKeyTheEffectiveResolver(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		args []string
		want string
	}{
		{"the default is typed", []string{"--repo", "/src"}, "typed"},
		{"typed spelled out", []string{"--repo", "/src", "--resolver", "typed"}, "typed"},
		{"lsp", []string{"--repo", "/src", "--resolver", "lsp"}, "lsp"},
	}

	keyed := map[string][]plancache.Flag{}

	for _, testCase := range testCases {
		set, _ := testSet(t, testCase.args...)
		got := Flags(set, nil)
		keyed[testCase.name] = got

		index := slices.IndexFunc(
			got,
			func(flag plancache.Flag) bool { return flag.Name == "resolver" },
		)
		if index < 0 || got[index].Value != testCase.want {
			t.Fatalf("%s: Flags() = %+v, want resolver %q", testCase.name, got, testCase.want)
		}
	}

	if !reflect.DeepEqual(keyed["the default is typed"], keyed["typed spelled out"]) {
		t.Fatalf("default %+v and explicit typed %+v key differently",
			keyed["the default is typed"], keyed["typed spelled out"])
	}

	if reflect.DeepEqual(keyed["typed spelled out"], keyed["lsp"]) {
		t.Fatalf("lsp and typed key alike: %+v", keyed["lsp"])
	}
}
