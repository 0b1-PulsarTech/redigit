package session

import (
	"context"
	"errors"
	"flag"
	"slices"
	"strconv"
	"testing"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
)

func testEnv() Env {
	return Env{
		BuildID: func() (string, error) { return "build-1", nil },
		Commit:  func(context.Context, string, string) (string, error) { return "commit-1", nil },
		Digest:  func(command string) (string, error) { return "digest-of-" + command, nil },
		Toolchain: func(_ context.Context, repo, commit string) (string, error) {
			return "toolchain-of-" + repo + "@" + commit, nil
		},
	}
}

type keyOutcome string

const (
	keySame      keyOutcome = "same"
	keyDifferent keyOutcome = "different"
	keyFails     keyOutcome = "fails"
)

type cacheKeyCase struct {
	name    string
	args    []string
	mutate  func(*Env)
	setenv  string
	outcome keyOutcome
}

func cacheKeyCases() []cacheKeyCase {
	errBoom := errors.New("boom")
	base := []string{"--repo", "/src", "--tags", "integration"}
	keep := func(*Env) {}

	return []cacheKeyCase{
		{"nothing changes", base, keep, "", keySame},
		{"run-only flags", append([]string{"--timings", "--far", "9"}, base...), keep, "", keySame},
		{"another tag", []string{"--repo", "/src", "--tags", "other"}, keep, "", keyDifferent},
		{
			"another repo path",
			[]string{"--repo", "/else", "--tags", "integration"},
			keep,
			"",
			keyDifferent,
		},
		{
			"a virtual module",
			append([]string{"--virtual", "virtual:x=web/x.ts"}, base...),
			keep,
			"",
			keyDifferent,
		},
		{"another toolchain", base, func(env *Env) {
			env.Toolchain = func(context.Context, string, string) (string, error) { return "go-2", nil }
		}, "", keyDifferent},
		{"no toolchain", base, func(env *Env) {
			env.Toolchain = func(context.Context, string, string) (string, error) { return "", errBoom }
		}, "", keyFails},
		{"another build", base, func(env *Env) {
			env.BuildID = func() (string, error) { return "build-2", nil }
		}, "", keyDifferent},
		{"another commit", base, func(env *Env) {
			env.Commit = func(context.Context, string, string) (string, error) { return "commit-2", nil }
		}, "", keyDifferent},
		{"another language server", base, func(env *Env) {
			env.Digest = func(command string) (string, error) { return "new-" + command, nil }
		}, "", keyDifferent},
		{"no build id", base, func(env *Env) {
			env.BuildID = func() (string, error) { return "", errBoom }
		}, "", keyFails},
		{"unknown revision", base, func(env *Env) {
			env.Commit = func(context.Context, string, string) (string, error) { return "", errBoom }
		}, "", keyFails},
		{"no language server", base, func(env *Env) {
			env.Digest = func(string) (string, error) { return "", errBoom }
		}, "", keyFails},
	}
}

func TestCacheKeyFollowsEveryInput(t *testing.T) {
	baseSet, baseCfg := testSet(t, "--repo", "/src", "--tags", "integration")

	want, err := cacheKey(t.Context(), baseSet, baseCfg, testEnv())
	if err != nil {
		t.Fatalf("base cacheKey() err = %v", err)
	}

	for _, testCase := range cacheKeyCases() {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.setenv != "" {
				t.Setenv("GOOS", testCase.setenv)
			}

			env := testEnv()
			testCase.mutate(&env)

			set, cfg := testSet(t, testCase.args...)

			got, keyErr := cacheKey(t.Context(), set, cfg, env)

			switch testCase.outcome {
			case keyFails:
				if keyErr == nil {
					t.Fatal("cacheKey() err = nil, want a failure")
				}
			case keySame, keyDifferent:
				if keyErr != nil || (got == want) != (testCase.outcome == keySame) {
					t.Fatalf(
						"cacheKey() = %q, %v, want it %s from %q",
						got,
						keyErr,
						testCase.outcome,
						want,
					)
				}
			}
		})
	}
}

func TestCacheKeyHashesTheParsedConfigBytes(t *testing.T) {
	t.Parallel()

	keyWith := func(file string, parsed string) string {
		set, cfg := testSet(t, "--repo", "/src")

		if set.Lookup("config") == nil {
			set.String("config", "", "stand-in for the flag the config slice adds")
		}

		if err := set.Set("config", file); err != nil {
			t.Fatalf("set --config: %v", err)
		}

		cfg.ConfigBytes = []byte(parsed)

		key, err := cacheKey(t.Context(), set, cfg, testEnv())
		if err != nil {
			t.Fatalf("cacheKey() err = %v", err)
		}

		return key
	}

	first := keyWith("/absent/a.json", `{"a":1}`)

	if keyWith("/absent/b.json", `{"a":1}`) != first {
		t.Fatal("the key depends on the config file's path, not its parsed bytes")
	}

	if keyWith("/absent/a.json", `{"a":2}`) == first {
		t.Fatal("the key did not change when the parsed config bytes changed")
	}

	if keyWith("/absent/a.json", `{"a":1}`+"\n") == first {
		t.Fatal("the key ignored a trailing newline in the parsed config bytes")
	}
}

func TestCacheKeyFailsOnAnUnreadableFileFlag(t *testing.T) {
	t.Parallel()

	set, cfg := testSet(t, "--repo", "/src")

	if err := set.Set("chunk-map", "/nonexistent/chunks.json"); err != nil {
		t.Fatalf("set --chunk-map: %v", err)
	}

	if key, err := cacheKey(t.Context(), set, cfg, testEnv()); err == nil || key != "" {
		t.Fatalf("cacheKey() = %q, %v, want an error and no key", key, err)
	}
}

func cacheKey(
	ctx context.Context,
	set *flag.FlagSet,
	cfg bootstrap.Config,
	env Env,
) (string, error) {
	resolved, err := Resolve(ctx, set, cfg, env)

	return resolved.Key, err
}

func TestWithCacheResolvesTheRevOnce(t *testing.T) {
	t.Parallel()

	set, cfg := testSet(t, "--repo", "/src", "--rev", "main")
	dir := t.TempDir()
	lookups := 0

	var seen []string

	env := testEnv()
	env.CacheDir = dir
	env.Commit = func(context.Context, string, string) (string, error) {
		lookups++

		return "oid-" + strconv.Itoa(lookups), nil
	}
	env.Start = func(_ context.Context, run bootstrap.Config) (Pipeline, error) {
		seen = append(seen, run.Rev)

		return nil, errors.New("stop here")
	}

	if _, err := WithCache(t.Context(), set, cfg, env)(t.Context(), cfg); err == nil {
		t.Fatal("start err = nil, want the stub's failure")
	}

	if lookups != 1 || !slices.Equal(seen, []string{"oid-1"}) {
		t.Fatalf("%d lookups and a run on %q, want one lookup and a run on oid-1", lookups, seen)
	}
}

func TestResolveAsksEveryProbeAboutThisRunsInputs(t *testing.T) {
	t.Parallel()

	set, cfg := testSet(t, "--repo", "/src", "--rev", "topic")
	cfg.ServerCommand = "other-server"

	var got []string

	env := testEnv()
	env.Commit = func(_ context.Context, repo, rev string) (string, error) {
		got = append(got, "commit:"+repo+"@"+rev)

		return "oid", nil
	}
	env.Digest = func(command string) (string, error) {
		got = append(got, "digest:"+command)

		return "d", nil
	}
	env.Toolchain = func(_ context.Context, repo, commit string) (string, error) {
		got = append(got, "toolchain:"+repo+"@"+commit)

		return "t", nil
	}

	if _, err := Resolve(t.Context(), set, cfg, env); err != nil {
		t.Fatalf("Resolve() err = %v", err)
	}

	want := []string{"commit:/src@topic", "digest:other-server", "toolchain:/src@oid"}
	if !slices.Equal(got, want) {
		t.Fatalf("probes were asked %v, want %v", got, want)
	}
}

func TestCacheKeyFollowsTheConfigsServerArgsAndVirtuals(t *testing.T) {
	t.Parallel()

	set, cfg := testSet(t, "--repo", "/src")

	base, err := cacheKey(t.Context(), set, cfg, testEnv())
	if err != nil {
		t.Fatalf("base cacheKey() err = %v", err)
	}

	testCases := []struct {
		name   string
		mutate func(*bootstrap.Config)
	}{
		{"other server args", func(cfg *bootstrap.Config) {
			cfg.ServerArgs = []string{"-rpc.trace"}
		}},
		{"a virtual module", func(cfg *bootstrap.Config) {
			cfg.Virtual = map[string]string{"virtual:x": "web/x.ts"}
		}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			changed := cfg
			testCase.mutate(&changed)

			got, keyErr := cacheKey(t.Context(), set, changed, testEnv())
			if keyErr != nil || got == base {
				t.Fatalf("cacheKey() = %q, %v, want a key other than %q", got, keyErr, base)
			}
		})
	}
}
