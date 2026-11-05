package lint

import (
	"errors"
	"flag"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/transport/cli/write"
)

func TestParseRefusesAPipelineFlagWithoutARepo(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{name: "no pipeline flag at all", args: []string{"--manifest", "m"}},
		{name: "a repo alone", args: []string{"--manifest", "m", "--repo", "/r"}},
		{
			name: "a rev with a repo",
			args: []string{"--manifest", "m", "--repo", "/r", "--rev", "v"},
		},
		{
			name:    "a rev without a repo",
			args:    []string{"--manifest", "m", "--rev", "v"},
			wantErr: true,
		},
		{
			name:    "tags without a repo",
			args:    []string{"--manifest", "m", "--tags", "a"},
			wantErr: true,
		},
		{
			name:    "a mode without a repo",
			args:    []string{"--manifest", "m", "--mode", "lineage"},
			wantErr: true,
		},
		{
			name:    "a resolver without a repo",
			args:    []string{"--manifest", "m", "--resolver", "typed"},
			wantErr: true,
		},
		{
			name:    "from-cache without a repo",
			args:    []string{"--manifest", "m", "--from-cache"},
			wantErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := parse(testCase.args)
			if errors.Is(err, write.ErrUsage) != testCase.wantErr ||
				(err != nil) != testCase.wantErr {
				t.Fatalf("parse() err = %v, want a usage error: %v", err, testCase.wantErr)
			}
		})
	}
}

func TestPipelineConfig(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "redigit.json")
	if err := os.WriteFile(configPath, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("WriteFile() err = %v", err)
	}

	file, err := bootstrap.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("LoadConfigFile() err = %v", err)
	}

	testCases := []struct {
		name     string
		args     []string
		file     *bootstrap.ConfigFile
		wantMode string
		wantTags int
		wantPath string
		wantErr  bool
	}{
		{
			name:     "the flags land in the config",
			args:     []string{"--repo", "/r", "--rev", "v1", "--tags", "a,b", "--mode", "lineage"},
			wantMode: bootstrap.ModeLineage, wantTags: 2,
		},
		{
			name: "the defaults are the pipeline's own", args: []string{"--repo", "/r"},
			wantMode: bootstrap.ModeHistory,
		},
		{
			name: "the config file is applied", args: []string{"--repo", "/r"}, file: &file,
			wantMode: bootstrap.ModeHistory, wantPath: configPath,
		},
		{
			name:    "a mode that does not exist",
			args:    []string{"--repo", "/r", "--mode", "nope"},
			wantErr: true,
		},
		{
			name:    "a resolver that does not exist",
			args:    []string{"--repo", "/r", "--resolver", "nope"},
			wantErr: true,
		},
		{
			name:    "tags that are not text",
			args:    []string{"--repo", "/r", "--tags", "\xff"},
			wantErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			opts, parseErr := parse(append([]string{"--manifest", "m"}, testCase.args...))
			if parseErr != nil {
				t.Fatalf("parse() err = %v", parseErr)
			}

			cfg, err := pipelineConfig(opts, testCase.file)
			if (err != nil) != testCase.wantErr ||
				errors.Is(err, write.ErrUsage) != (testCase.wantErr && testCase.file == nil) {
				t.Fatalf("pipelineConfig() err = %v, want an error: %v", err, testCase.wantErr)
			}

			if testCase.wantErr {
				return
			}

			if cfg.Mode != testCase.wantMode || len(cfg.BuildTags) != testCase.wantTags ||
				cfg.ConfigPath != testCase.wantPath || cfg.Repo != "/r" {
				t.Fatalf("pipelineConfig() = %+v", cfg)
			}
		})
	}
}

func TestKeyFlagsHoldWhatThePipelineSeesAndNothingOfLints(t *testing.T) {
	t.Parallel()

	opts, err := parse([]string{
		"--manifest", "m", "--blob-dir", "b", "--fail-over", "--config", "c.json",
		"--repo", "/r", "--rev", "v1", "--from-cache",
	})
	if err != nil {
		t.Fatalf("parse() err = %v", err)
	}

	got := map[string]string{}

	keyFlags(opts).Visit(func(visited *flag.Flag) { got[visited.Name] = visited.Value.String() })

	want := map[string]string{
		"config": "c.json", "repo": "/r", "rev": "v1", "from-cache": "true",
	}

	if !maps.Equal(got, want) {
		t.Fatalf("keyFlags() = %v, want %v", got, want)
	}
}
