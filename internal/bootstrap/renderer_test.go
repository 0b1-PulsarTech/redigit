package bootstrap

import (
	"context"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// stubRenderer reports which renderer was reached, which is the whole of what
// dispatch has to get right.
type stubRenderer struct{ tag string }

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

// The dispatch table and the language registration must never disagree about
// which files the config pack covers: a file indexed by it but rendered by the
// Go syntax comes back empty.
func TestConfigRenderersCoverEveryConfigExtension(t *testing.T) {
	t.Parallel()

	table := configRenderers(stubRenderer{tag: "config"})

	for _, ext := range configExtensions() {
		if _, found := table[ext]; !found {
			t.Fatalf("%q is indexed by the config pack and has no renderer", ext)
		}
	}

	if len(table) != len(configExtensions()) {
		t.Fatalf(
			"the table holds %d entries for %d extensions",
			len(table),
			len(configExtensions()),
		)
	}
}

func TestRenderersByExtensionFallsBackToConfig(t *testing.T) {
	t.Parallel()

	goSyntax := stubRenderer{tag: "go"}
	dartSyntax := stubRenderer{tag: "dart"}
	configSyntax := stubRenderer{tag: "config"}

	byExt := renderersByExtension(goSyntax, dartSyntax, configSyntax)
	dispatch := newRenderer(configSyntax, byExt)

	testCases := []struct {
		name string
		file string
		want string
	}{
		{"a Go file reads with the Go syntax", "apps/lead/main.go", "go"},
		{"a Dart file reads with the Dart syntax", "web/src/routes/+page.server.dart", "dart"},
		{"a known config extension reads with the config syntax", "apps/lead/go.mod", "config"},
		{
			"an extension neither pack lists falls back to config, not go",
			"engines/wwebjs/src/main.ts",
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
