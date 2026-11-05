package resume

import (
	"flag"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/infra/anchorstate"
)

type scanCase struct {
	name    string
	args    []string
	want    target
	wantErr bool
}

func TestScan(t *testing.T) {
	t.Parallel()

	checkScan(t, []scanCase{
		{
			name: "two dashes and a space",
			args: []string{"--repo", "r"},
			want: target{repo: "r", rev: "HEAD", end: 2},
		},
		{
			name: "one dash",
			args: []string{"-repo", "r"},
			want: target{repo: "r", rev: "HEAD", end: 2},
		},
		{
			name: "an equals sign",
			args: []string{"--repo=r"},
			want: target{repo: "r", rev: "HEAD", end: 1},
		},
		{
			name: "one dash and an equals sign",
			args: []string{"-config=x"},
			want: target{config: "x", rev: "HEAD", end: 1},
		},
		{
			name: "an equals sign and nothing after it",
			args: []string{"--config="},
			want: target{rev: "HEAD", end: 1},
		},
		{
			name: "the last one wins",
			args: []string{"--repo", "a", "--repo=b"},
			want: target{repo: "b", rev: "HEAD", end: 3},
		},
		{
			name: "every flag the resume needs",
			args: []string{"--repo", "r", "--rev", "v", "--config=c", "--state-dir", "d"},
			want: target{repo: "r", rev: "v", config: "c", stateDir: "d", end: 7},
		},
	})
}

func TestScanSkipsTheFlagsItDoesNotRead(t *testing.T) {
	t.Parallel()

	checkScan(t, []scanCase{
		{
			name: "an empty rev is the rev, as the flag takes it",
			args: []string{"--rev", ""},
			want: target{end: 2},
		},
		{
			name: "a value that starts with a dash",
			args: []string{"--config", "-x"},
			want: target{config: "-x", rev: "HEAD", end: 2},
		},
		{
			name: "two dashes as the value of a flag",
			args: []string{"--repo", "--", "--config", "c"},
			want: target{repo: "--", config: "c", rev: "HEAD", end: 4},
		},
		{
			name: "two dashes as a value and then as the end",
			args: []string{"--repo", "--", "--", "--config", "c"},
			want: target{repo: "--", rev: "HEAD", end: 2},
		},
		{
			name: "a flag the resume does not read",
			args: []string{"--tags", "x"},
			want: target{rev: "HEAD", end: 2},
		},
		{
			name: "a value that looks like a flag name belongs to the flag before it",
			args: []string{"--tags", "--config", "c"},
			want: target{rev: "HEAD", end: 2},
		},
		{
			name: "a flag that takes no value leaves the next one alone",
			args: []string{"--timings", "--config", "c"},
			want: target{config: "c", rev: "HEAD", end: 3},
		},
		{
			name: "a bool flag with a value",
			args: []string{"--from-cache=true", "--config", "c"},
			want: target{config: "c", rev: "HEAD", end: 3},
		},
		{
			name: "a bool flag switched off",
			args: []string{"--timings=false", "--config", "c"},
			want: target{config: "c", rev: "HEAD", end: 3},
		},
	})
}

func TestScanStopsWhereTheFlagsEnd(t *testing.T) {
	t.Parallel()

	checkScan(t, []scanCase{
		{
			name: "a positional argument ends the flags",
			args: []string{"--repo", "r", "extra", "--config", "c"},
			want: target{repo: "r", rev: "HEAD", end: 2},
		},
		{
			name: "a flag after a stray argument is not read",
			args: []string{"stray", "--state-dir", "d"},
			want: target{rev: "HEAD"},
		},
		{
			name: "a lone dash is positional",
			args: []string{"--repo", "r", "-", "--config", "c"},
			want: target{repo: "r", rev: "HEAD", end: 2},
		},
		{
			name: "two dashes end the flags",
			args: []string{"--repo", "r", "--", "--config", "c"},
			want: target{repo: "r", rev: "HEAD", end: 2},
		},
		{name: "nothing", args: nil, want: target{rev: "HEAD"}},
		{name: "a flag with no value", args: []string{"--repo"}, wantErr: true},
		{name: "a flag nobody defined", args: []string{"--nope", "x"}, wantErr: true},
		{name: "a malformed name", args: []string{"--repo", "r", "---x"}, wantErr: true},
		{name: "a request for help", args: []string{"-h"}, wantErr: true},
	})
}

func checkScan(t *testing.T, testCases []scanCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := scan(testFlags, testCase.args)
			if (err != nil) != testCase.wantErr || (err == nil && got != testCase.want) {
				t.Fatalf("scan(%q) = %+v, %v, want %+v (error %v)",
					testCase.args, got, err, testCase.want, testCase.wantErr)
			}
		})
	}
}

func TestScanWantsTheFlagsItReads(t *testing.T) {
	t.Parallel()

	bare := func() *flag.FlagSet { return flag.NewFlagSet("bare", flag.ContinueOnError) }

	_, err := scan(bare, nil)
	if err == nil {
		t.Fatal("scan() = nil, want a flag set that lacks --repo refused")
	}
}

func TestWithConfig(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "the config goes after the flags",
			args: []string{"emit", "--repo", "r"},
			want: []string{"emit", "--repo", "r", "--config", "new.json"},
		},
		{
			name: "the config goes before a positional argument",
			args: []string{"emit", "--repo", "r", "extra"},
			want: []string{"emit", "--repo", "r", "--config", "new.json", "extra"},
		},
		{
			name: "the config goes before the terminator",
			args: []string{"emit", "--repo", "r", "--", "--config", "x"},
			want: []string{"emit", "--repo", "r", "--config", "new.json", "--", "--config", "x"},
		},
		{
			name: "an earlier config stays and the new one comes last",
			args: []string{"emit", "--config=old.json", "--repo", "r"},
			want: []string{
				"emit", "--config=old.json", "--repo", "r", "--config", "new.json",
			},
		},
		{
			name: "a flag value that looks like a flag is not taken for one",
			args: []string{"emit", "--tags", "--config", "old.json"},
			want: []string{"emit", "--tags", "--config", "--config", "new.json", "old.json"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			original := slices.Clone(testCase.args)

			found, err := scan(testFlags, testCase.args[1:])
			if err != nil {
				t.Fatalf("scan() = %v", err)
			}

			got := found.withConfig(testCase.args, "new.json")
			if !slices.Equal(got, testCase.want) || !slices.Equal(testCase.args, original) {
				t.Fatalf("withConfig() = %q, want %q (input %q)", got, testCase.want, testCase.args)
			}
		})
	}
}

type derivedCase struct {
	name   string
	config func(t *testing.T, dir, derived string) string
	want   bool
}

func TestIsDerived(t *testing.T) {
	t.Parallel()

	checkIsDerived(t, []derivedCase{
		{
			name: "the derived file itself",
			config: func(_ *testing.T, _, derived string) string {
				return derived
			},
			want: true,
		},
		{
			name: "the derived file by another spelling",
			config: func(_ *testing.T, dir, _ string) string {
				return filepath.Join(dir, ".", "sub", "..", anchorstate.DerivedFile)
			},
			want: true,
		},
		{
			name: "another file with the same content",
			config: func(t *testing.T, dir, _ string) string {
				t.Helper()

				other := filepath.Join(dir, "other.json")
				if err := os.WriteFile(other, []byte("{}"), 0o600); err != nil {
					t.Fatal(err)
				}

				return other
			},
		},
		{
			name: "a config that is not there",
			config: func(_ *testing.T, dir, _ string) string {
				return filepath.Join(dir, "missing.json")
			},
		},
	})
}

func TestIsDerivedFollowsLinks(t *testing.T) {
	t.Parallel()

	checkIsDerived(t, []derivedCase{
		{
			name: "a hard link to the derived file",
			config: func(t *testing.T, dir, derived string) string {
				t.Helper()

				link := filepath.Join(dir, "hard.json")
				if err := os.Link(derived, link); err != nil {
					t.Fatal(err)
				}

				return link
			},
			want: true,
		},
		{
			name: "a symbolic link to the derived file",
			config: func(t *testing.T, dir, derived string) string {
				t.Helper()

				link := filepath.Join(dir, "soft.json")
				if err := os.Symlink(derived, link); err != nil {
					t.Fatal(err)
				}

				return link
			},
			want: true,
		},
	})
}

func checkIsDerived(t *testing.T, testCases []derivedCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			derived := filepath.Join(dir, anchorstate.DerivedFile)

			if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o750); err != nil {
				t.Fatal(err)
			}

			if err := os.WriteFile(derived, []byte("{}"), 0o600); err != nil {
				t.Fatal(err)
			}

			found := target{config: testCase.config(t, dir, derived)}
			if got := found.isDerived(dir); got != testCase.want {
				t.Fatalf("isDerived() = %v, want %v for %s", got, testCase.want, found.config)
			}
		})
	}
}
