package golang

import (
	"context"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestBuildConstraints(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	writeFile(t, root, "linux_only.go", "//go:build linux\n\npackage sample\n")
	writeFile(t, root, "plain.go", "package sample\n")
	writeFile(t, root, "legacy.go", "// +build integration\n\npackage sample\n")
	writeFile(t, root, "combo.go", "//go:build linux && (amd64 || arm64)\n\npackage sample\n")

	snap := entities.Snapshot{
		Root:  root,
		Files: []string{"linux_only.go", "plain.go", "legacy.go", "combo.go"},
	}

	got, err := BuildConstraints(snap)
	if err != nil {
		t.Fatalf("BuildConstraints() error = %v", err)
	}

	byFile := map[string]BuildTag{}
	for _, tag := range got {
		byFile[tag.File] = tag
	}

	testCases := []struct {
		name     string
		file     string
		wantTags []string
	}{
		{"go:build tag", "linux_only.go", []string{"linux"}},
		{"legacy +build tag", "legacy.go", []string{"integration"}},
		{"compound expression collects every tag", "combo.go", []string{"amd64", "arm64", "linux"}},
	}

	if _, untagged := byFile["plain.go"]; untagged {
		t.Fatalf("plain.go has no build constraint; it must not appear in the report")
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			tag, found := byFile[testCase.file]
			if !found {
				t.Fatalf("no BuildTag for %s; got %+v", testCase.file, got)
			}

			gotTags := append([]string{}, tag.Tags...)
			sort.Strings(gotTags)

			if len(gotTags) != len(testCase.wantTags) {
				t.Fatalf("Tags = %v, want %v", gotTags, testCase.wantTags)
			}

			for i, want := range testCase.wantTags {
				if gotTags[i] != want {
					t.Fatalf("Tags = %v, want %v", gotTags, testCase.wantTags)
				}
			}
		})
	}
}

func TestBuildConstraintsReadError(t *testing.T) {
	t.Parallel()

	snap := entities.Snapshot{Root: t.TempDir(), Files: []string{"missing.go"}}

	_, err := BuildConstraints(snap)
	if err == nil {
		t.Fatal("BuildConstraints() error = nil, want an error for a file that does not exist")
	}
}

func TestBuildTagsRuleContributesNoEdgesOrAnchors(t *testing.T) {
	t.Parallel()

	edges, err := buildTagsRule{}.Edges(context.Background(), entities.Snapshot{}, nil)
	if err != nil {
		t.Fatalf("Edges() error = %v", err)
	}

	if len(edges) != 0 {
		t.Fatalf("got %d edges, want 0", len(edges))
	}

	anchors, err := buildTagsRule{}.Anchors(context.Background(), entities.Snapshot{}, nil)
	if err != nil {
		t.Fatalf("Anchors() error = %v", err)
	}

	if len(anchors) != 0 {
		t.Fatalf("got %d anchors, want 0", len(anchors))
	}
}

func TestBuildConstraintsFileInSubdir(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	writeFile(t, root, filepath.Join("pkg", "windows.go"), "//go:build windows\n\npackage pkg\n")

	snap := entities.Snapshot{Root: root, Files: []string{filepath.Join("pkg", "windows.go")}}

	got, err := BuildConstraints(snap)
	if err != nil {
		t.Fatalf("BuildConstraints() error = %v", err)
	}

	if len(got) != 1 || got[0].File != filepath.Join("pkg", "windows.go") {
		t.Fatalf("got %+v, want one entry for the subdirectory file", got)
	}
}

func TestHidden(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		tags      []BuildTag
		satisfied []string
		wantFiles []string
	}{
		{
			name:      "constraint the run satisfies is not hidden",
			tags:      []BuildTag{{File: "a_test.go", Constraint: "//go:build integration"}},
			satisfied: []string{"integration"},
			wantFiles: nil,
		},
		{
			name:      "constraint the run does not satisfy is hidden",
			tags:      []BuildTag{{File: "a_test.go", Constraint: "//go:build integration"}},
			satisfied: nil,
			wantFiles: []string{"a_test.go"},
		},
		{
			name:      "current platform satisfies its own goos",
			tags:      []BuildTag{{File: "p.go", Constraint: "//go:build " + runtime.GOOS}},
			satisfied: nil,
			wantFiles: nil,
		},
		{
			name:      "negated tag is hidden once the tag is passed",
			tags:      []BuildTag{{File: "n.go", Constraint: "//go:build !integration"}},
			satisfied: []string{"integration"},
			wantFiles: []string{"n.go"},
		},
		{
			name: "only the unsatisfied half of a mixed set is reported",
			tags: []BuildTag{
				{File: "seen.go", Constraint: "//go:build integration"},
				{File: "unseen.go", Constraint: "//go:build e2e"},
			},
			satisfied: []string{"integration"},
			wantFiles: []string{"unseen.go"},
		},
		{
			name:      "an and expression needs every tag",
			tags:      []BuildTag{{File: "both.go", Constraint: "//go:build integration && e2e"}},
			satisfied: []string{"integration"},
			wantFiles: []string{"both.go"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := Hidden(testCase.tags, testCase.satisfied)
			if err != nil {
				t.Fatalf("Hidden: %v", err)
			}

			files := make([]string, 0, len(got))
			for _, tag := range got {
				files = append(files, tag.File)
			}

			if !slices.Equal(files, testCase.wantFiles) {
				t.Fatalf("hidden files = %v, want %v", files, testCase.wantFiles)
			}
		})
	}
}

func TestHiddenRejectsAMalformedConstraint(t *testing.T) {
	t.Parallel()

	_, err := Hidden([]BuildTag{{File: "bad.go", Constraint: "//go:build ("}}, nil)
	if err == nil {
		t.Fatal("Hidden accepted a constraint that does not parse")
	}
}
