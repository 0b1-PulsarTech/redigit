package write

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
)

const fullConfig = `{"write":{"identity":{"name":"Cfg","email":"cfg@x"},"dateRef":"cfgref","subjects":"cfg.json"}}`

func resolveWith(t *testing.T, config string, opts options) (options, error) {
	t.Helper()

	if config != "" {
		opts.configPath = filepath.Join(t.TempDir(), "redigit.json")
		if err := os.WriteFile(opts.configPath, []byte(config), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
	}

	got, err := resolve(opts)
	got.configPath = ""

	return got, err
}

func TestResolveMerges(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		config string
		opts   options
		want   options
	}{
		{
			"everything from the config",
			fullConfig,
			options{},
			options{
				authorName:  "Cfg",
				authorEmail: "cfg@x",
				dateRef:     "cfgref",
				subjects:    "cfg.json",
			},
		},
		{
			"flags win over the config",
			fullConfig,
			options{authorName: "Flag", dateRef: "flagref", subjects: "flag.json"},
			options{
				authorName:  "Flag",
				authorEmail: "cfg@x",
				dateRef:     "flagref",
				subjects:    "flag.json",
			},
		},
		{
			"no config at all needs the flags", "",
			options{authorName: "n", authorEmail: "e", dateRef: "d"},
			options{authorName: "n", authorEmail: "e", dateRef: "d"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveWith(t, testCase.config, testCase.opts)
			if err != nil || got != testCase.want {
				t.Fatalf("resolve() = %+v, %v, want %+v", got, err, testCase.want)
			}
		})
	}
}

func TestResolveRejects(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		config  string
		wantErr error
	}{
		{"missing identity", `{"write":{"dateRef":"d"}}`, ErrUsage},
		{"missing email", `{"write":{"identity":{"name":"n"},"dateRef":"d"}}`, ErrUsage},
		{"missing date ref", `{"write":{"identity":{"name":"n","email":"e"}}}`, ErrUsage},
		{"newline in the name", identityConfig("a\\nb", "e"), ErrUsage},
		{"newline in the email", identityConfig("n", "e\\nf"), ErrUsage},
		{"angle in the name", identityConfig("a<b", "e"), ErrUsage},
		{"angle in the email", identityConfig("n", "e>f"), ErrUsage},
		{"NUL in the name", identityConfig("a\\u0000b", "e"), ErrUsage},
		{"tab in the email", identityConfig("n", "e\\tf"), ErrUsage},
		{"escape byte in the name", identityConfig("a\\u001bb", "e"), ErrUsage},
		{"unknown write key", `{"write":{"colour":"red"}}`, bootstrap.ErrConfig},
		{"unknown identity key", `{"write":{"identity":{"nick":"x"}}}`, bootstrap.ErrConfig},
		{"wrong value type", `{"write":{"dateRef":3}}`, bootstrap.ErrConfig},
		{"write section that is not an object", `{"write":[]}`, bootstrap.ErrConfig},
		{"null write section", `{"write":null}`, bootstrap.ErrConfig},
		{"config file that does not exist", "", bootstrap.ErrConfig},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			opts := options{}
			if testCase.config == "" {
				opts.configPath = filepath.Join(t.TempDir(), "absent.json")
			}

			_, err := resolveWith(t, testCase.config, opts)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("resolve() err = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func identityConfig(name, email string) string {
	return `{"write":{"identity":{"name":"` + name + `","email":"` + email + `"},"dateRef":"d"}}`
}

func TestResolveRefusesDashRevisions(t *testing.T) {
	t.Parallel()

	base := options{authorName: "n", authorEmail: "e", dateRef: "d"}

	testCases := []struct {
		name   string
		config string
		edit   func(opts *options)
	}{
		{"verify from a flag", "", func(opts *options) { opts.verify = "--output=x" }},
		{"date ref from a flag", "", func(opts *options) { opts.dateRef = "--output=x" }},
		{
			"date ref from the config",
			`{"write":{"dateRef":"--output=x"}}`,
			func(opts *options) { opts.dateRef = "" },
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			opts := base
			testCase.edit(&opts)

			_, err := resolveWith(t, testCase.config, opts)
			if !errors.Is(err, ErrUsage) {
				t.Fatalf("resolve() err = %v, want ErrUsage", err)
			}
		})
	}
}
