package session

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
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
		"memprofile", "trace", "mem-limit",
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
		{Name: "repo", Value: "/src"},
		{Name: "tags", Value: "a,b"},
	}
	if got := Flags(set, nil); !reflect.DeepEqual(got, want) {
		t.Fatalf("Flags() = %+v, want %+v", got, want)
	}
}
