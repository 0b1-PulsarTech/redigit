package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/infra/inputs"
	"github.com/wrapped-owls/redigit/internal/infra/rules"
)

func TestParseConfigFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tagged := writeIn(t, dir, "tagged.json", `{"languages":{"go":{"tags":["integration"]},`+
		`"web":{"virtual":{"virtual:a":"file.dart","virtual:b":"b.dart"}}}}`)

	fileVirtual := map[string]string{"virtual:a": "file.dart", "virtual:b": "b.dart"}

	testCases := []struct {
		name        string
		args        []string
		wantTags    []string
		wantVirtual map[string]string
	}{
		{
			"the file sets the tags",
			[]string{"--repo", ".", "--config", tagged},
			[]string{"integration"},
			fileVirtual,
		},
		{
			"a tags flag overrides the file",
			[]string{"--repo", ".", "--config", tagged, "--tags", "other"},
			[]string{"other"},
			fileVirtual,
		},
		{
			"a virtual flag replaces the whole file map",
			[]string{"--repo", ".", "--config", tagged, "--virtual", "virtual:a=flag.dart"},
			[]string{"integration"},
			map[string]string{"virtual:a": "flag.dart"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg, _, err := parse(newFlagSet(), testCase.args)
			if err != nil {
				t.Fatalf("parse() = %v", err)
			}

			if !slices.Equal(cfg.BuildTags, testCase.wantTags) {
				t.Fatalf("BuildTags = %v, want %v", cfg.BuildTags, testCase.wantTags)
			}

			if !maps.Equal(cfg.Virtual, testCase.wantVirtual) {
				t.Fatalf("Virtual = %v, want %v", cfg.Virtual, testCase.wantVirtual)
			}
		})
	}
}

func TestParseConfigFileKeepsTheCause(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	missing := filepath.Join(dir, "no.json")
	unknown := writeIn(t, dir, "unknown.json", `{"rules":[{"type":"go-di"}]}`)
	malformed := writeIn(t, dir, "malformed.json", `{"languages":`)
	language := writeIn(t, dir, "language.json", `{"languages":{"golang":{}}}`)

	testCases := []struct {
		name      string
		path      string
		wantInner error
		wantText  string
	}{
		{"a missing file", missing, os.ErrNotExist, ": no such file or directory"},
		{
			"an unknown rule type", unknown, rules.ErrUnknownType,
			`: rules[0]: unknown type "go-di" (no rule types are registered in this build)`,
		},
		{"a malformed file", malformed, rules.ErrMalformed, ": invalid JSON at offset 13"},
		{
			"an unknown language", language, bootstrap.ErrUnknownLanguage,
			`: unknown language "golang" (settings exist for: go, web)`,
		},
		{"a directory", dir, rules.ErrNotRegular, ": not a regular file"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := parse(newFlagSet(), []string{"--repo", ".", "--config", testCase.path})
			if !errors.Is(err, bootstrap.ErrConfig) || !errors.Is(err, testCase.wantInner) {
				t.Fatalf("err = %v, want ErrConfig and the cause %v", err, testCase.wantInner)
			}

			if want := "config file " + testCase.path + testCase.wantText; err.Error() != want {
				t.Fatalf("err = %q, want one message %q", err, want)
			}
		})
	}
}

func writeIn(t testing.TB, dir, name, content string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}

	return path
}

type (
	virtualPair struct{ id, path string }

	virtualCase struct {
		name    string
		pairs   []virtualPair
		want    map[string]string
		wantErr error
		wantMsg string
	}
)

func virtualCases() []virtualCase {
	const (
		badPairs = "languages.web.virtual: expected ID to PATH pairs, both non-empty"
		twice    = "languages.web.virtual: virtual ID listed twice"
		notUTF8  = "languages.web.virtual: text holds U+FFFD or invalid UTF-8"
	)

	empty := inputs.ErrVirtual

	return []virtualCase{
		{
			"one pair",
			[]virtualPair{{"virtual:a", "a.dart"}},
			map[string]string{"virtual:a": "a.dart"},
			nil,
			"",
		},
		{"blanks are trimmed", []virtualPair{{" a ", " x "}}, map[string]string{"a": "x"}, nil, ""},
		{
			"two pairs",
			[]virtualPair{{"a", "x"}, {"b", "y"}},
			map[string]string{"a": "x", "b": "y"},
			nil,
			"",
		},
		{"an empty id", []virtualPair{{"", "x"}}, nil, empty, badPairs},
		{"an empty path", []virtualPair{{"a", ""}}, nil, empty, badPairs},
		{"a blank id and path", []virtualPair{{" ", " "}}, nil, empty, badPairs},
		{
			"a good pair then an empty id",
			[]virtualPair{{"a", "x"}, {"", "y"}},
			nil,
			empty,
			badPairs,
		},
		{
			"an id twice once trimmed",
			[]virtualPair{{"a", "x"}, {" a ", "y"}},
			nil,
			inputs.ErrDuplicateVirtual, twice,
		},
		{
			"a replacement character in the id",
			[]virtualPair{{"a�", "x"}},
			nil,
			inputs.ErrNotUTF8,
			notUTF8,
		},
		{
			"a replacement character in the path",
			[]virtualPair{{"a", "�"}},
			nil,
			inputs.ErrNotUTF8,
			notUTF8,
		},
		{
			"a byte that is no UTF-8",
			[]virtualPair{{"a", "x\xff"}},
			nil,
			inputs.ErrNotUTF8,
			notUTF8,
		},
	}
}

func TestVirtualFlagAndFileAgree(t *testing.T) {
	t.Parallel()

	for _, testCase := range virtualCases() {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			args := make([]string, 0, 2+2*len(testCase.pairs))
			args = append(args, "--repo", ".")
			members := make([]string, 0, len(testCase.pairs))

			for _, entry := range testCase.pairs {
				args = append(args, "--virtual", entry.id+"="+entry.path)

				id, _ := json.Marshal(entry.id)
				path, _ := json.Marshal(entry.path)
				members = append(members, string(id)+":"+string(path))
			}

			path := writeIn(t, t.TempDir(), "virtual.json",
				`{"languages":{"web":{"virtual":{`+strings.Join(members, ",")+`}}}}`)

			fromFlag, _, flagErr := parse(newFlagSet(), args)
			fromFile, _, fileErr := parse(newFlagSet(), []string{"--repo", ".", "--config", path})

			if !errors.Is(flagErr, testCase.wantErr) || !errors.Is(fileErr, testCase.wantErr) {
				t.Fatalf(
					"flag err = %v, file err = %v, want %v",
					flagErr,
					fileErr,
					testCase.wantErr,
				)
			}

			if testCase.wantErr != nil {
				if want := "config file " + path + ": " + testCase.wantMsg; fileErr.Error() != want {
					t.Fatalf("file err = %q, want %q", fileErr, want)
				}

				return
			}

			if !maps.Equal(fromFlag.Virtual, testCase.want) ||
				!maps.Equal(fromFile.Virtual, testCase.want) {
				t.Fatalf(
					"flag %v, file %v, want %v",
					fromFlag.Virtual,
					fromFile.Virtual,
					testCase.want,
				)
			}
		})
	}
}

func TestTagsFlagAndFileAgree(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		tags    []string
		want    []string
		wantErr error
	}{
		{"one tag", []string{"integration"}, []string{"integration"}, nil},
		{"blanks are trimmed", []string{" a ", "b"}, []string{"a", "b"}, nil},
		{"a replacement character", []string{"a�"}, nil, inputs.ErrNotUTF8},
		{"a byte that is no UTF-8", []string{"a\xff"}, nil, inputs.ErrNotUTF8},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			quoted, _ := json.Marshal(testCase.tags)
			path := writeIn(t, t.TempDir(), "tags.json",
				`{"languages":{"go":{"tags":`+string(quoted)+`}}}`)

			fromFlag, _, flagErr := parse(
				newFlagSet(), []string{"--repo", ".", "--tags", strings.Join(testCase.tags, ",")},
			)
			fromFile, _, fileErr := parse(newFlagSet(), []string{"--repo", ".", "--config", path})

			if !errors.Is(flagErr, testCase.wantErr) || !errors.Is(fileErr, testCase.wantErr) {
				t.Fatalf(
					"flag err = %v, file err = %v, want both %v",
					flagErr,
					fileErr,
					testCase.wantErr,
				)
			}

			if !slices.Equal(fromFlag.BuildTags, testCase.want) ||
				!slices.Equal(fromFile.BuildTags, testCase.want) {
				t.Fatalf(
					"flag %v, file %v, want both %v",
					fromFlag.BuildTags,
					fromFile.BuildTags,
					testCase.want,
				)
			}
		})
	}
}

func TestParseRefusesAnEmptyConfigPath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{"an empty value", []string{"--repo", ".", "--config", ""}, true},
		{"an empty value with an equals sign", []string{"--repo", ".", "--config="}, true},
		{"no flag at all", []string{"--repo", "."}, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := parse(newFlagSet(), testCase.args)
			if got := errors.Is(err, bootstrap.ErrConfig); got != testCase.wantErr {
				t.Fatalf("parse() err = %v, want ErrConfig %v", err, testCase.wantErr)
			}

			if err == nil && testCase.wantErr {
				t.Fatal("parse() = nil, want an error")
			}
		})
	}
}

func TestDispatchFromCacheKeysTheConfigBytes(t *testing.T) {
	t.Parallel()

	const original = `{"languages":{"go":{"tags":["integration"]}}}`

	testCases := []struct {
		name       string
		rewrite    string
		wantStarts int
	}{
		{name: "identical file hits", rewrite: original, wantStarts: 1},
		{
			name: "changed file misses", rewrite: `{"languages":{"go":{"tags":["other"]}}}`,
			wantStarts: 2,
		},
		{name: "trailing newline misses", rewrite: original + "\n", wantStarts: 2},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			path := writeIn(t, t.TempDir(), "c.json", original)
			counter := &startCounter{live: settledLive(t)}
			env := cacheTestEnv(t.TempDir(), counter.start)
			args := []string{cmdGraph, "--repo", ".", "--config", path, "--from-cache"}

			cachedRun(t, env, args...)

			if err := os.WriteFile(path, []byte(testCase.rewrite), 0o600); err != nil {
				t.Fatalf("rewrite config: %v", err)
			}

			cachedRun(t, env, args...)

			if counter.count != testCase.wantStarts {
				t.Fatalf("%d pipeline starts, want %d", counter.count, testCase.wantStarts)
			}
		})
	}
}

func TestDispatchPrintsAConfigErrorAlone(t *testing.T) {
	t.Parallel()

	errStart := fmt.Errorf("%w: config file c.json: rules[0]", bootstrap.ErrConfig)
	absent := filepath.Join(t.TempDir(), "absent.json")

	unreachable := func(context.Context, bootstrap.Config) (pipeline, error) {
		t.Fatal("a load-time config error must never start a pipeline")

		return nil, nil
	}

	testCases := []struct {
		name  string
		args  []string
		start starter
		want  string
	}{
		{
			name:  "refused while parsing",
			args:  []string{cmdGraph, "--repo", ".", "--config", absent},
			start: unreachable,
			want:  "config file ",
		},
		{
			name:  "refused when the pipeline starts",
			args:  []string{cmdGraph, "--repo", "."},
			start: func(context.Context, bootstrap.Config) (pipeline, error) { return nil, errStart },
			want:  errStart.Error(),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer

			env := runEnv{Start: testCase.start}

			err := dispatch(t.Context(), testCase.args, &stdout, &stderr, env)
			if !errors.Is(err, bootstrap.ErrConfig) ||
				!strings.HasPrefix(err.Error(), testCase.want) {
				t.Fatalf("err = %v, want ErrConfig starting %q", err, testCase.want)
			}

			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want no usage dump over a config error", stderr.String())
			}
		})
	}
}
