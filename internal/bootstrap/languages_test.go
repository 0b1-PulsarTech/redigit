package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/bootstrap/languages"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/web"
	"github.com/wrapped-owls/redigit/internal/infra/sliceconf"
)

// stubRenderer reports which renderer was reached, which is the whole of what
// dispatch has to get right.
type stubRenderer struct{ tag string }

func (stubRenderer) Declarations(context.Context, string, []byte) ([]entities.Decl, error) {
	return nil, nil
}

func (s stubRenderer) Render(
	_ context.Context,
	_ []byte,
	_ []entities.DeclKey,
) ([]byte, error) {
	return []byte(s.tag), nil
}

func TestRendererDispatchesByExtension(t *testing.T) {
	t.Parallel()

	dispatch := newRenderer(
		stubRenderer{tag: "go"},
		map[string]renderer{".mod": stubRenderer{tag: "config"}},
	)

	testCases := []struct {
		name string
		keep []entities.DeclKey
		want string
	}{
		{
			// A go.mod rendered by the Go syntax parses as nothing and comes
			// back empty, and the manifest still carries a blob: the empty
			// file. Silent, and only visible as a tree that diverges.
			name: "a file the config pack owns",
			keep: []entities.DeclKey{{File: "apps/lead/go.mod", Name: "m"}},
			want: "config",
		},
		{
			name: "a Go file",
			keep: []entities.DeclKey{{File: "apps/lead/main.go", Name: "main"}},
			want: "go",
		},
		{
			name: "an extension nothing claims",
			keep: []entities.DeclKey{{File: "README.md", Name: "<file>"}},
			want: "go",
		},
		{
			name: "nothing kept",
			keep: nil,
			want: "go",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := dispatch.Render(t.Context(), nil, testCase.keep)
			if err != nil {
				t.Fatalf("Render() = %v", err)
			}

			if string(got) != testCase.want {
				t.Fatalf("reached the %q renderer, want %q", got, testCase.want)
			}
		})
	}
}

// The dispatch table is read off the language registration, so the two can
// never disagree about which syntax wrote a file it indexed.
func TestRenderersByExtensionFallsBackToConfig(t *testing.T) {
	t.Parallel()

	configSyntax := stubRenderer{tag: "config"}
	byExt := renderersByExtension([]indexing.Language{
		{Name: "go", Extensions: []string{".go"}, Syntax: stubRenderer{tag: "go"}},
		{Name: "dart", Extensions: []string{".dart"}, Syntax: stubRenderer{tag: "dart"}},
		{Name: "web", Extensions: web.Extensions(), Syntax: stubRenderer{tag: "web"}},
		{Name: "config", Extensions: languages.ConfigExtensions(), Syntax: configSyntax},
	})
	dispatch := newRenderer(configSyntax, byExt)

	testCases := []struct {
		name string
		file string
		want string
	}{
		{"a Go file reads with the Go syntax", "apps/lead/main.go", "go"},
		{"a Dart file reads with the Dart syntax", "web/src/routes/+page.server.dart", "dart"},
		{"a known config extension reads with the config syntax", "apps/lead/go.mod", "config"},
		{"a Svelte component reads with the web syntax", "web/src/routes/+page.svelte", "web"},
		{
			"an extension no pack lists falls back to config, not go",
			"engines/wwebjs/src/main.tmpl",
			"config",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			keep := []entities.DeclKey{{File: testCase.file, Name: "<file>"}}

			got, err := dispatch.Render(t.Context(), nil, keep)
			if err != nil {
				t.Fatalf("Render() = %v", err)
			}

			if string(got) != testCase.want {
				t.Fatalf("reached the %q renderer, want %q", got, testCase.want)
			}
		})
	}
}

func TestDecodeSliceReadsTheSectionOrRefusesItNamingTheFile(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		section       string
		wantFunctions []string
		wantErr       error
	}{
		{name: "no section is the zero config", section: ""},
		{
			name:          "a section is decoded",
			section:       `{"functions":["Run"],"errcheck":true}`,
			wantFunctions: []string{"Run"},
		},
		{
			name:    "an unknown key is refused as a config error",
			section: `{"functions":["Run"],"tails":true}`,
			wantErr: ErrConfig,
		},
		{
			name:    "an invalid section also matches the section's own error",
			section: `{"functions":[""]}`,
			wantErr: sliceconf.ErrInvalid,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := Config{ConfigPath: "redigit.json"}
			cfg.Sections.Slice = json.RawMessage(testCase.section)

			got, err := decodeSlice(cfg)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("decodeSlice() error = %v, want %v", err, testCase.wantErr)
			}

			if err != nil && !strings.Contains(err.Error(), "redigit.json") {
				t.Fatalf("decodeSlice() error = %q, want the config path named", err)
			}

			if !slices.Equal(got.Functions, testCase.wantFunctions) {
				t.Fatalf("decodeSlice() = %v, want %v", got.Functions, testCase.wantFunctions)
			}
		})
	}
}

func TestStartRefusesABadSliceSectionBeforeOpeningTheRepository(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		section string
	}{
		{name: "an unknown key", section: `{"functions":["Run"],"tails":true}`},
		{name: "a null section", section: `null`},
		{name: "a bad glob", section: `{"functions":[""]}`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := Config{Repo: t.TempDir() + "/absent", ConfigPath: "redigit.json"}
			cfg.Sections.Slice = json.RawMessage(testCase.section)

			pipe, err := start(context.Background(), cfg, nil)
			if !errors.Is(err, sliceconf.ErrInvalid) || !errors.Is(err, ErrConfig) {
				t.Fatalf("start() error = %v, want the section's error and ErrConfig", err)
			}

			if pipe != nil {
				t.Fatalf("start() = %v, want no pipeline", pipe)
			}
		})
	}
}
