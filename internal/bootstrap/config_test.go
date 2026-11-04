package bootstrap

import (
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/wrapped-owls/redigit/internal/infra/langpack/config"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
	"github.com/wrapped-owls/redigit/internal/infra/rules"
	"github.com/wrapped-owls/redigit/internal/infra/sliceconf"
)

// TestDefaults pins the values a run starts from before flags narrow it. The
// tool that shipped without them is why this file exists: ordering.Config had
// no defaults, so RoundLimit arrived as 0 and the fixed-point loop never ran,
// handing back an ordering that was nothing but its own seed while looking
// settled.
func TestDefaults(t *testing.T) {
	t.Parallel()

	got := Defaults()

	testCases := []struct {
		name string
		got  any
		want any
	}{
		{"rev defaults to HEAD", got.Rev, "HEAD"},
		{"the server command is gopls", got.ServerCommand, "gopls"},
		{"the mode is the published history's", got.Mode, ModeHistory},
		{"the author name is redigit", got.Author.Name, "redigit"},
		{"the author email is redigit@localhost", got.Author.Email, "redigit@localhost"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if testCase.got != testCase.want {
				t.Fatalf("got %v, want %v", testCase.got, testCase.want)
			}
		})
	}

	if want := []string{"-mode=stdio"}; len(got.ServerArgs) != 1 || got.ServerArgs[0] != want[0] {
		t.Fatalf("ServerArgs = %v, want %v", got.ServerArgs, want)
	}

	if !got.Author.Start.Equal(time.Unix(0, 0).UTC()) {
		t.Fatalf("Author.Start = %v, want the unix epoch", got.Author.Start)
	}

	// Repo and Rev.BuildTags are left zero on purpose: a default repository
	// path would silently point a run at the wrong tree.
	if got.Repo != "" {
		t.Fatalf("Repo = %q, want empty so --repo is required", got.Repo)
	}

	if got.BuildTags != nil {
		t.Fatalf("BuildTags = %v, want nil", got.BuildTags)
	}
}

func TestCheckMode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		mode    string
		wantErr error
	}{
		{"history", ModeHistory, nil},
		{"lineage", ModeLineage, nil},
		{"empty is refused", "", ErrMode},
		{"unknown is refused", "fold", ErrMode},
		{"case matters", "Lineage", ErrMode},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if err := CheckMode(testCase.mode); !errors.Is(err, testCase.wantErr) {
				t.Fatalf("CheckMode(%q) = %v, want %v", testCase.mode, err, testCase.wantErr)
			}
		})
	}
}

func TestSettingsMatchThePacks(t *testing.T) {
	t.Parallel()

	withSettings := map[string]bool{}
	for _, entry := range settings() {
		withSettings[entry.language] = true
	}

	packsWithoutSettings := []string{dartLanguage, configLanguage}

	packs := make([]string, 0, 4)
	goSource := typed.NewSource(nil)

	for _, lang := range specificLanguages(config.Syntax{}, finalTree{}, goSource, sliceconf.Config{}) {
		packs = append(packs, lang.Name)
	}

	for _, name := range packs {
		if withSettings[name] == slices.Contains(packsWithoutSettings, name) {
			t.Fatalf("pack %q: want it in exactly one of settings() and packsWithoutSettings", name)
		}
	}

	for name := range withSettings {
		if !slices.Contains(packs, name) {
			t.Fatalf("settings() names %q, which is no pack", name)
		}
	}

	if withSettings[opaqueName] || slices.Contains(packs, opaqueName) {
		t.Fatalf("the catch-all %q must not take settings", opaqueName)
	}
}

func TestSettingsTableIsComplete(t *testing.T) {
	t.Parallel()

	seen := map[string]bool{}

	for _, entry := range settings() {
		id := entry.language + "." + entry.key

		if entry.language == "" || entry.key == "" || entry.flag == "" || entry.apply == nil {
			t.Fatalf("setting %q: language, key, flag and apply must all be set", id)
		}

		if seen[id] {
			t.Fatalf("setting %q is listed twice", id)
		}

		seen[id] = true
	}
}

func TestSettingsDecode(t *testing.T) {
	t.Parallel()

	testCases := settingCases()

	covered := map[string]bool{}

	for _, testCase := range testCases {
		covered[testCase.language+"."+testCase.key] = true

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := loadAndApply(t, testCase.content, testCase.prior, testCase.explicit)

			if !slices.Equal(got.BuildTags, testCase.wantTags) {
				t.Fatalf("BuildTags = %q, want %q", got.BuildTags, testCase.wantTags)
			}

			if !maps.Equal(got.Virtual, testCase.wantVirtuals) {
				t.Fatalf("Virtual = %v, want %v", got.Virtual, testCase.wantVirtuals)
			}
		})
	}

	for _, entry := range settings() {
		if id := entry.language + "." + entry.key; !covered[id] {
			t.Fatalf("setting %q has no decoded-JSON case in TestSettingsDecode", id)
		}
	}
}

func TestApplyConfigFileFillsWhatFlagsLeftAlone(t *testing.T) {
	t.Parallel()

	content := `{"languages":{"go":{"tags":["integration"]},` +
		`"web":{"virtual":{"virtual:a":"from-file"}}}}`

	got := loadAndApply(t, content, Config{}, nil)

	if !slices.Equal(got.BuildTags, []string{"integration"}) ||
		!maps.Equal(got.Virtual, map[string]string{"virtual:a": "from-file"}) {
		t.Fatalf("got tags %q virtual %v", got.BuildTags, got.Virtual)
	}
}

func TestApplyConfigFileRefusesWhatLoadWouldHave(t *testing.T) {
	t.Parallel()

	languages := map[string]map[string]json.RawMessage{
		"go": {"tags": json.RawMessage(`null`)},
	}
	file := ConfigFile{Path: "x.json", File: rules.File{Languages: languages}}

	_, err := ApplyConfigFile(Config{}, file, func(string) bool { return false })

	want := "config file x.json: languages.go.tags: expected an array of strings"
	if !errors.Is(err, ErrConfig) || err == nil || err.Error() != want {
		t.Fatalf("ApplyConfigFile() err = %v, want %q", err, want)
	}
}

func loadAndApply(t testing.TB, content string, prior Config, explicit map[string]bool) Config {
	t.Helper()

	file, err := LoadConfigFile(writeConfigFile(t, content))
	if err != nil {
		t.Fatalf("LoadConfigFile() = %v", err)
	}

	got, err := ApplyConfigFile(prior, file, func(name string) bool { return explicit[name] })
	if err != nil {
		t.Fatalf("ApplyConfigFile() = %v", err)
	}

	return got
}

type settingCase struct {
	name         string
	language     string
	key          string
	content      string
	explicit     map[string]bool
	prior        Config
	wantTags     []string
	wantVirtuals map[string]string
}

func settingCases() []settingCase {
	return []settingCase{
		{
			name: "go tags reach BuildTags", language: goLanguage, key: "tags",
			content:  `{"languages":{"go":{"tags":["integration"]}}}`,
			wantTags: []string{"integration"},
		},
		{
			name: "a padded tag is trimmed", language: goLanguage, key: "tags",
			content:  `{"languages":{"go":{"tags":[" integration "]}}}`,
			wantTags: []string{"integration"},
		},
		{
			name: "an empty tag is dropped", language: goLanguage, key: "tags",
			content:  `{"languages":{"go":{"tags":["a","","  ","b"]}}}`,
			wantTags: []string{"a", "b"},
		},
		{
			name: "a comma splits as it does on the command line", language: goLanguage,
			key: "tags", content: `{"languages":{"go":{"tags":["a,b"]}}}`,
			wantTags: []string{"a", "b"},
		},
		{
			name: "only blanks leave no tags", language: goLanguage, key: "tags",
			content: `{"languages":{"go":{"tags":["", " "]}}}`,
		},
		{
			name: "an explicit tags flag wins, even when empty", language: goLanguage,
			key: "tags", content: `{"languages":{"go":{"tags":["integration"]}}}`,
			explicit: map[string]bool{"tags": true},
		},
		{
			name: "web virtual reaches Virtual", language: webLanguage, key: "virtual",
			content:      `{"languages":{"web":{"virtual":{"virtual:a":"a.dart"}}}}`,
			wantVirtuals: map[string]string{"virtual:a": "a.dart"},
		},
		{
			name: "an explicit virtual flag replaces the whole file map", language: webLanguage,
			key:          "virtual",
			content:      `{"languages":{"web":{"virtual":{"virtual:a":"from-file"}}}}`,
			explicit:     map[string]bool{"virtual": true},
			prior:        Config{Virtual: map[string]string{"virtual:a": "from-flag"}},
			wantVirtuals: map[string]string{"virtual:a": "from-flag"},
		},
	}
}
