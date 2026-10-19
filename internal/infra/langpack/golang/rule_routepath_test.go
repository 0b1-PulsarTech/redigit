package golang

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestRoutePathRuleAnchors(t *testing.T) {
	t.Parallel()

	files := routeFixture()
	root := t.TempDir()
	paths := make([]string, 0, len(files))

	for name, content := range files {
		writeFile(t, root, name, content)

		paths = append(paths, name)
	}

	decls := declareAll(t, NewSyntax(), root, paths)

	anchors, err := routePathRule{}.Anchors(
		context.Background(), entities.Snapshot{Root: root}, decls,
	)
	if err != nil {
		t.Fatalf("Anchors() error = %v", err)
	}

	got := map[string][]string{}
	for _, anchor := range anchors {
		got[anchor.Decl.Name] = append(got[anchor.Decl.Name], anchor.To.File)
	}

	testCases := []struct {
		test string
		want string
	}{
		{test: "TestGet", want: filepath.Join("ctl", "conversation", "handler.go")},
		{test: "TestAppearance", want: filepath.Join("ctl", "appearance", "handler.go")},
		{test: "TestSettings", want: filepath.Join("ctl", "settings", "handler.go")},
		{test: "TestNoRoute", want: ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.test, func(t *testing.T) {
			t.Parallel()

			files := got[testCase.test]
			if testCase.want == "" {
				if len(files) != 0 {
					t.Fatalf("anchored to %v, want none", files)
				}

				return
			}

			if len(files) != 1 || files[0] != testCase.want {
				t.Fatalf("anchored to %v, want [%s]", files, testCase.want)
			}
		})
	}
}

func routeFixture() map[string]string {
	return map[string]string{
		filepath.Join("ctl", "conversation", "handler.go"): `package conversation

type Handlers struct{}

func (Handlers) GroupName() string { return "/conversations" }
`,
		filepath.Join("ctl", "settings", "handler.go"): `package settings

type Handlers struct{}

func (Handlers) GroupName() string { return "/settings" }
`,
		filepath.Join("ctl", "appearance", "handler.go"): `package appearance

type Handlers struct{}

func (Handlers) GroupName() string { return "/settings/appearance" }
`,
		filepath.Join("e2e", "flow_test.go"): `package e2e

import "testing"

func TestGet(t *testing.T) { _ = "/conversations/{id}/messages?limit=5" }

func TestAppearance(t *testing.T) { _ = "/settings/appearance/theme" }

func TestSettings(t *testing.T) { _ = "/settings/schedule" }

func TestNoRoute(t *testing.T) { _ = "not/a/path"; _ = "/ unknown" }
`,
	}
}
