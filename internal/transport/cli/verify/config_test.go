package verify

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
	"github.com/wrapped-owls/redigit/internal/infra/checkrun"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "redigit.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	return path
}

func resolveWith(t *testing.T, body string, args ...string) (plan, error) {
	t.Helper()

	if body != "" {
		args = append([]string{"--config", writeConfig(t, body)}, args...)
	}

	opts, err := parse(args)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	return resolve(opts)
}

func TestResolve(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		config    string
		args      []string
		wantOpts  verifying.Options
		wantBuild []string
	}{
		{
			"defaults", "", nil,
			verifying.Options{Ref: "HEAD", Stride: 1, Parallel: 2},
			[]string{"go", "build", "./..."},
		},
		{
			"flags", "",
			[]string{
				"--tags", "a,b", "--exclude", "x,y", "--stride", "5", "--parallel", "3",
				"--from", "2", "--to", "8", "--ref", "main",
			},
			verifying.Options{
				Ref: "main", From: 2, To: 8, Stride: 5, Parallel: 3, Exclude: []string{"x", "y"},
			},
			[]string{"go", "build", "-tags", "a,b", "./..."},
		},
		{
			"config only",
			`{"verify":{"tags":["integration"],"parallel":4,"stride":7,"exclude":["tools"]}}`,
			nil,
			verifying.Options{Ref: "HEAD", Stride: 7, Parallel: 4, Exclude: []string{"tools"}},
			[]string{"go", "build", "-tags", "integration", "./..."},
		},
		{
			"flags win over config",
			`{"verify":{"tags":["integration"],"parallel":4,"stride":7,"exclude":["tools"]}}`,
			[]string{"--tags", "x", "--parallel", "1", "--stride", "2", "--exclude", "y"},
			verifying.Options{Ref: "HEAD", Stride: 2, Parallel: 1, Exclude: []string{"y"}},
			[]string{"go", "build", "-tags", "x", "./..."},
		},
		{
			"config without a verify section", `{}`, nil,
			verifying.Options{Ref: "HEAD", Stride: 1, Parallel: 2},
			[]string{"go", "build", "./..."},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveWith(t, testCase.config, testCase.args...)
			if err != nil {
				t.Fatalf("resolve() err = %v", err)
			}

			if !reflect.DeepEqual(got.opts, testCase.wantOpts) {
				t.Fatalf("opts = %+v, want %+v", got.opts, testCase.wantOpts)
			}

			if build := got.languages["go"].Steps[0].Argv; !slices.Equal(
				build,
				testCase.wantBuild,
			) {
				t.Fatalf("go build argv = %v, want %v", build, testCase.wantBuild)
			}

			if got.markers["go.mod"] != "go" {
				t.Fatalf("markers = %v, want go.mod -> go", got.markers)
			}
		})
	}
}

func TestResolveLanguages(t *testing.T) {
	t.Parallel()

	config := `{"verify":{"languages":{"rust":{"marker":"Cargo.toml",
		"version":["rustc","--version"],"env":["RUSTFLAGS","CARGO_BUILD_TARGET"],
		"steps":[{"cmd":["cargo","build"],"timeout":"5m"},
		{"cmd":["cargo","clippy"],"tolerate":["no targets"]}]}}}}`

	got, err := resolveWith(t, config)
	if err != nil {
		t.Fatalf("resolve() err = %v", err)
	}

	want := checkrun.Language{
		Steps: []checkrun.Step{
			{Argv: []string{"cargo", "build"}, Timeout: 5 * time.Minute},
			{Argv: []string{"cargo", "clippy"}, Tolerate: []string{"no targets"}},
		},
		Version: []string{"rustc", "--version"},
		Env:     []string{"RUSTFLAGS", "CARGO_BUILD_TARGET"},
	}

	if !reflect.DeepEqual(got.languages["rust"], want) {
		t.Fatalf("rust = %+v, want %+v", got.languages["rust"], want)
	}

	if got.markers["Cargo.toml"] != "rust" || got.markers["go.mod"] != "go" {
		t.Fatalf("markers = %v", got.markers)
	}
}

func TestResolveRejectsBadConfig(t *testing.T) {
	t.Parallel()

	rust := func(body string) string {
		return `{"verify":{"languages":{"rust":` + body + `}}}`
	}

	testCases := []struct {
		name   string
		config string
	}{
		{"unknown section key", `{"verify":{"nope":1}}`},
		{"section not an object", `{"verify":[]}`},
		{"section null", `{"verify":null}`},
		{"parallel wrong type", `{"verify":{"parallel":"2"}}`},
		{"parallel fractional", `{"verify":{"parallel":1.5}}`},
		{"parallel zero", `{"verify":{"parallel":0}}`},
		{"parallel above the cpu count", `{"verify":{"parallel":100000}}`},
		{"stride negative", `{"verify":{"stride":-1}}`},
		{"tags wrong type", `{"verify":{"tags":"a"}}`},
		{"exclude wrong type", `{"verify":{"exclude":[1]}}`},
		{"exclude empty entry", `{"verify":{"exclude":[""]}}`},
		{"exclude dot entry", `{"verify":{"exclude":["."]}}`},
		{"exclude dot slash beside a dir", `{"verify":{"exclude":["tools","./"]}}`},
		{"languages not an object", `{"verify":{"languages":[]}}`},
		{"language not an object", rust(`[]`)},
		{"unknown language key", rust(`{"marker":"m","steps":[{"cmd":["x"]}],"nope":1}`)},
		{"missing marker", rust(`{"steps":[{"cmd":["x"]}]}`)},
		{"marker with a slash", rust(`{"marker":"a/b","steps":[{"cmd":["x"]}]}`)},
		{"missing steps", rust(`{"marker":"m"}`)},
		{"empty steps", rust(`{"marker":"m","steps":[]}`)},
		{"step not an object", rust(`{"marker":"m","steps":["x"]}`)},
		{"step unknown key", rust(`{"marker":"m","steps":[{"cmd":["x"],"nope":1}]}`)},
		{"step without cmd", rust(`{"marker":"m","steps":[{"timeout":"1m"}]}`)},
		{"step empty cmd", rust(`{"marker":"m","steps":[{"cmd":[]}]}`)},
		{"step bad timeout", rust(`{"marker":"m","steps":[{"cmd":["x"],"timeout":"soon"}]}`)},
		{"step zero timeout", rust(`{"marker":"m","steps":[{"cmd":["x"],"timeout":"0s"}]}`)},
		{"step timeout wrong type", rust(`{"marker":"m","steps":[{"cmd":["x"],"timeout":5}]}`)},
		{"step tolerate wrong type", rust(`{"marker":"m","steps":[{"cmd":["x"],"tolerate":"y"}]}`)},
		{"version wrong type", rust(`{"marker":"m","steps":[{"cmd":["x"]}],"version":"v"}`)},
		{"env wrong type", rust(`{"marker":"m","steps":[{"cmd":["x"]}],"env":"RUSTFLAGS"}`)},
		{
			"marker taken by go",
			rust(`{"marker":"go.mod","steps":[{"cmd":["x"]}]}`),
		},
		{"malformed file", `{`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := resolveWith(t, testCase.config)
			if !errors.Is(err, bootstrap.ErrConfig) {
				t.Fatalf("resolve() err = %v, want ErrConfig", err)
			}
		})
	}
}

func TestResolveTagsAgainstGoLanguage(t *testing.T) {
	t.Parallel()

	goSteps := `"go":{"steps":[{"cmd":["go","build","./..."]}]}`

	testCases := []struct {
		name    string
		config  string
		args    []string
		wantErr bool
	}{
		{
			"tags in the config beside languages.go",
			`{"verify":{"tags":["a"],"languages":{` + goSteps + `}}}`, nil, true,
		},
		{
			"the tags flag beside languages.go",
			`{"verify":{"languages":{` + goSteps + `}}}`,
			[]string{"--tags", "a"},
			true,
		},
		{
			"languages.go without tags",
			`{"verify":{"languages":{` + goSteps + `}}}`, nil, false,
		},
		{
			"tags without languages.go",
			`{"verify":{"tags":["a"]}}`, nil, false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := resolveWith(t, testCase.config, testCase.args...)
			if gotErr := errors.Is(err, bootstrap.ErrConfig); gotErr != testCase.wantErr ||
				(err != nil) != testCase.wantErr {
				t.Fatalf("resolve() err = %v, want a config error %v", err, testCase.wantErr)
			}
		})
	}
}

func TestResolveMissingConfigFile(t *testing.T) {
	t.Parallel()

	opts, err := parse([]string{"--config", filepath.Join(t.TempDir(), "absent.json")})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if _, err = resolve(opts); !errors.Is(err, bootstrap.ErrConfig) {
		t.Fatalf("resolve() err = %v, want ErrConfig", err)
	}
}
