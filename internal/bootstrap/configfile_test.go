package bootstrap

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/rules"
)

func writeConfigFile(t testing.TB, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "redigit.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	return path
}

func TestLoadConfigFile(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		content string
		missing bool
		wantErr error
	}{
		{"an empty object", `{}`, false, nil},
		{"an object after a BOM", "\xEF\xBB\xBF{}", false, nil},
		{"settings only", `{"languages":{"go":{"tags":["integration"]}}}`, false, nil},
		{"a missing file", ``, true, os.ErrNotExist},
		{"malformed json", `{`, false, rules.ErrMalformed},
		{"an unknown key", `{"nope":1}`, false, rules.ErrMalformed},
		{"an unknown rule type", `{"rules":[{"type":"go-di"}]}`, false, rules.ErrUnknownType},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "absent.json")
			if !testCase.missing {
				path = writeConfigFile(t, testCase.content)
			}

			_, err := LoadConfigFile(path)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("LoadConfigFile() err = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

var errBroken = errors.New("broken on purpose")

func globRegistry() rules.Registry {
	return rules.NewRegistry(map[string]rules.Constructor{
		"broken-rule": func(json.RawMessage) (rules.Rule, error) { return nil, errBroken },
		"glob-rule": func(raw json.RawMessage) (rules.Rule, error) {
			var body struct {
				Paths []string `json:"paths"`
			}

			if err := json.Unmarshal(raw, &body); err != nil {
				return nil, fmt.Errorf("decode glob rule: %w", err)
			}

			return globRule{body.Paths}, nil
		},
	})
}

func TestLoadConfigFileRendersOneMessage(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		content string
		wantErr error
		want    string
	}{
		{"malformed json", `{`, rules.ErrMalformed, `invalid JSON at offset 1`},
		{
			"an unknown top-level key", `{"nope":1}`, rules.ErrMalformed,
			`unknown field "nope"`,
		},
		{
			"a rule without a type", `{"rules":[{"type":"glob-rule"},{}]}`,
			rules.ErrMissingType, `rules[1]: missing type`,
		},
		{
			"a type that is no string", `{"rules":[{"type":7}]}`, rules.ErrMalformed,
			`rules[0].type: expected a string`,
		},
		{
			"an unknown type", `{"rules":[{"type":"glob-rule"},{"type":"nope"}]}`,
			rules.ErrUnknownType, `rules[1]: unknown type "nope" (known: broken-rule, glob-rule)`,
		},
		{
			"a malformed glob",
			`{"rules":[{"type":"glob-rule","paths":["a"]},{"type":"glob-rule","paths":["[a"]}]}`,
			rules.ErrBadGlob,
			`malformed glob: rules[1] (glob-rule) has "[a": syntax error in pattern`,
		},
		{
			"a failing constructor", `{"rules":[{"type":"broken-rule"}]}`,
			errBroken, `rules[0] (broken-rule): broken on purpose`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			path := writeConfigFile(t, testCase.content)

			_, err := loadConfigFile(path, globRegistry())
			if !errors.Is(err, ErrConfig) || !errors.Is(err, testCase.wantErr) {
				t.Fatalf("loadConfigFile() err = %v, want ErrConfig and %v", err, testCase.wantErr)
			}

			want := "config file " + path + ": " + testCase.want
			if err.Error() != want {
				t.Fatalf("loadConfigFile() err = %q, want %q", err, want)
			}
		})
	}
}

func TestLoadConfigFileRendersWithoutRuleTypes(t *testing.T) {
	t.Parallel()

	path := writeConfigFile(t, `{"rules":[{"type":"go-di"}]}`)

	_, err := LoadConfigFile(path)

	want := "config file " + path +
		`: rules[0]: unknown type "go-di" (no rule types are registered in this build)`
	if !errors.Is(err, ErrConfig) || !errors.Is(err, rules.ErrUnknownType) || err.Error() != want {
		t.Fatalf("LoadConfigFile() err = %v, want %q", err, want)
	}
}

func TestLoadConfigFileLanguages(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		content string
		wantErr error
		wantMsg string
	}{
		{"the languages that read settings", `{"languages":{"go":{},"web":{}}}`, nil, ""},
		{
			"every setting a language reads",
			`{"languages":{"go":{"tags":["x"]},"web":{"virtual":{"virtual:a":"a"}}}}`, nil, "",
		},
		{
			"a misspelt language", `{"languages":{"go":{},"golang":{"tags":["x"]}}}`,
			ErrUnknownLanguage, `unknown language "golang" (settings exist for: go, web)`,
		},
		{
			"a pack that reads no settings", `{"languages":{"dart":{}}}`,
			ErrUnknownLanguage, `unknown language "dart" (settings exist for: go, web)`,
		},
		{
			"the catch-all pack", `{"languages":{"opaque":{}}}`,
			ErrUnknownLanguage, `unknown language "opaque" (settings exist for: go, web)`,
		},
		{
			"a misspelt setting", `{"languages":{"go":{"tagz":["x"]}}}`,
			ErrUnknownSetting, `language "go": unknown setting "tagz" (known: tags)`,
		},
		{
			"a setting another language reads", `{"languages":{"web":{"tags":["x"]}}}`,
			ErrUnknownSetting, `language "web": unknown setting "tags" (known: virtual)`,
		},
		{
			"a go virtual map", `{"languages":{"go":{"virtual":{}}}}`,
			ErrUnknownSetting, `language "go": unknown setting "virtual" (known: tags)`,
		},
		{
			"a lone surrogate tag", `{"languages":{"go":{"tags":["\ud800"]}}}`,
			ErrNotUTF8, `languages.go.tags: text holds U+FFFD or invalid UTF-8`,
		},
		{
			"a lone surrogate virtual path", `{"languages":{"web":{"virtual":{"a":"\ud800"}}}}`,
			ErrNotUTF8, `languages.web.virtual: text holds U+FFFD or invalid UTF-8`,
		},
		{
			"a lone surrogate virtual id", `{"languages":{"web":{"virtual":{"\udfff":"x"}}}}`,
			ErrNotUTF8, `languages.web.virtual: text holds U+FFFD or invalid UTF-8`,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			path := writeConfigFile(t, testCase.content)

			_, err := LoadConfigFile(path)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("LoadConfigFile() err = %v, want %v", err, testCase.wantErr)
			}

			if err == nil {
				return
			}

			if !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), path) {
				t.Fatalf("LoadConfigFile() err = %q, want ErrConfig and the path %q", err, path)
			}

			if want := "config file " + path + ": " + testCase.wantMsg; err.Error() != want {
				t.Fatalf("LoadConfigFile() err = %q, want %q", err, want)
			}
		})
	}
}

type globRule struct{ globs []string }

func (rule globRule) Type() string { return "glob-rule" }

func (rule globRule) Globs() []string { return rule.globs }

func TestValidateRules(t *testing.T) {
	t.Parallel()

	snap := entities.Snapshot{Files: []string{"database/migrations/001.sql", "main.go"}}

	testCases := []struct {
		name    string
		built   []rules.Rule
		wantErr error
		want    string
	}{
		{"no rules", nil, nil, ""},
		{"every glob matches", []rules.Rule{globRule{[]string{"*.go", "database/"}}}, nil, ""},
		{
			"a glob matching nothing",
			[]rules.Rule{globRule{[]string{"*.rs"}}},
			rules.ErrUnmatchedGlob,
			`config file /etc/redigit.json: glob matches nothing in the tree: ` +
				`rules[0] (glob-rule) has "*.rs"`,
		},
		{
			"a malformed glob",
			[]rules.Rule{globRule{[]string{"[a"}}},
			rules.ErrBadGlob,
			`config file /etc/redigit.json: malformed glob: ` +
				`rules[0] (glob-rule) has "[a": syntax error in pattern`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := Config{Rules: testCase.built, ConfigPath: "/etc/redigit.json"}

			err := validateRules(cfg, snap)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("validateRules() err = %v, want %v", err, testCase.wantErr)
			}

			if err == nil {
				return
			}

			if !errors.Is(err, ErrConfig) || err.Error() != testCase.want {
				t.Fatalf("validateRules() err = %q, want ErrConfig and %q", err, testCase.want)
			}
		})
	}
}

func TestLoadConfigFileSaysItInOurWords(t *testing.T) {
	t.Parallel()

	testCases := wordingCases()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			path := writeConfigFile(t, testCase.content)

			_, err := LoadConfigFile(path)
			if !errors.Is(err, ErrConfig) || !errors.Is(err, testCase.wantErr) {
				t.Fatalf("LoadConfigFile() err = %v, want ErrConfig and %v", err, testCase.wantErr)
			}

			if want := "config file " + path + ": " + testCase.want; err.Error() != want {
				t.Fatalf("LoadConfigFile() err = %q, want %q", err, want)
			}
		})
	}
}

type wordingCase struct {
	name    string
	content string
	wantErr error
	want    string
}

func wordingCases() []wordingCase {
	return []wordingCase{
		{"an empty file", ``, rules.ErrEmpty, `empty file`},
		{"whitespace only", " \n ", rules.ErrEmpty, `empty file`},
		{"a top-level null", `null`, rules.ErrMalformed, `expected a JSON object`},
		{
			"null languages", `{"languages":null}`, rules.ErrMalformed,
			`languages: expected an object`,
		},
		{
			"a null language", `{"languages":{"go":null}}`, rules.ErrMalformed,
			`languages.go: expected an object`,
		},
		{
			"null tags", `{"languages":{"go":{"tags":null}}}`, ErrConfig,
			`languages.go.tags: expected an array of strings`,
		},
		{
			"tags that are no array", `{"languages":{"go":{"tags":3}}}`, ErrConfig,
			`languages.go.tags: expected an array of strings`,
		},
		{
			"a tag that is no string", `{"languages":{"go":{"tags":["a",1]}}}`, ErrConfig,
			`languages.go.tags: expected an array of strings`,
		},
		{
			"a null tag", `{"languages":{"go":{"tags":[null]}}}`, ErrConfig,
			`languages.go.tags: expected an array of strings`,
		},
		{
			"null virtual", `{"languages":{"web":{"virtual":null}}}`, ErrConfig,
			`languages.web.virtual: expected an object of strings`,
		},
		{
			"a null virtual value", `{"languages":{"web":{"virtual":{"a":null}}}}`, ErrConfig,
			`languages.web.virtual: expected an object of strings`,
		},
		{
			"a virtual target that is no string", `{"languages":{"web":{"virtual":{"a":1}}}}`,
			ErrConfig, `languages.web.virtual: expected an object of strings`,
		},
		{
			"a misspelt setting", `{"languages":{"go":{"tagz":[]}}}`, ErrUnknownSetting,
			`language "go": unknown setting "tagz" (known: tags)`,
		},
		{
			"the first misspelt setting in order", `{"languages":{"go":{"zz":1,"aa":1}}}`,
			ErrUnknownSetting, `language "go": unknown setting "aa" (known: tags)`,
		},
		{
			"the first bad language in order", `{"languages":{"zz":{},"aa":{}}}`,
			ErrUnknownLanguage, `unknown language "aa" (settings exist for: go, web)`,
		},
		{"an unknown field", `{"foo":1}`, rules.ErrMalformed, `unknown field "foo"`},
		{
			"a rule type that is no string", `{"rules":[{"type":3}]}`, rules.ErrMalformed,
			`rules[0].type: expected a string`,
		},
		{"a rule without a type", `{"rules":[{}]}`, rules.ErrMissingType, `rules[0]: missing type`},
		{
			"a truncated object", `{"languages":{"go"`, rules.ErrMalformed,
			`invalid JSON at offset 18`,
		},
	}
}

func TestLoadConfigFileReportsTheFirstBadLanguageInOrder(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		content string
		wantMsg string
	}{
		{
			"two unknown languages", `{"languages":{"zed":{},"bar":{}}}`,
			`unknown language "bar" (settings exist for: go, web)`,
		},
		{
			"two unknown settings", `{"languages":{"go":{"zed":1,"bar":2}}}`,
			`language "go": unknown setting "bar" (known: tags)`,
		},
		{
			"two languages with bad values", `{"languages":{"web":{"virtual":1},"go":{"tags":1}}}`,
			`languages.go.tags: expected an array of strings`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			path := writeConfigFile(t, testCase.content)
			want := "config file " + path + ": " + testCase.wantMsg

			for range 64 {
				if _, err := LoadConfigFile(path); err == nil || err.Error() != want {
					t.Fatalf("LoadConfigFile() err = %v, want %q", err, want)
				}
			}
		})
	}
}

func TestApplyConfigFileKeepsRulesAndPath(t *testing.T) {
	t.Parallel()

	rule := globRule{globs: []string{"*.go"}}
	file := ConfigFile{Path: "/etc/redigit.json", Rules: []rules.Rule{rule}}

	cfg, err := ApplyConfigFile(Config{}, file, func(string) bool { return false })
	if err != nil {
		t.Fatalf("ApplyConfigFile() = %v", err)
	}

	if cfg.ConfigPath != file.Path {
		t.Fatalf("ConfigPath = %q, want %q", cfg.ConfigPath, file.Path)
	}

	if len(cfg.Rules) != 1 {
		t.Fatalf("Rules = %v, want one rule", cfg.Rules)
	}

	if got, ok := cfg.Rules[0].(globRule); !ok || got.Type() != rule.Type() {
		t.Fatalf("Rules[0] = %#v, want %#v", cfg.Rules[0], rule)
	}
}

func TestLoadConfigFileKeepsTheBytesItParsed(t *testing.T) {
	t.Parallel()

	const content = `{"languages":{"go":{"tags":["integration"]}}}` + "\n"

	path := writeConfigFile(t, content)

	file, err := LoadConfigFile(path)
	if err != nil {
		t.Fatalf("LoadConfigFile() = %v", err)
	}

	if err = os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("rewrite config: %v", err)
	}

	cfg, err := ApplyConfigFile(Config{}, file, func(string) bool { return false })
	if err != nil {
		t.Fatalf("ApplyConfigFile() = %v", err)
	}

	if string(cfg.ConfigBytes) != content {
		t.Fatalf("ConfigBytes = %q, want %q", cfg.ConfigBytes, content)
	}
}
