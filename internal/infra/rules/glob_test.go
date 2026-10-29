package rules

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	t.Parallel()

	tree := []string{
		"database/migrations/001.sql",
		"database/migrations/sub/002.sql",
		"docs/spec/a.md",
		"main.go",
	}

	testCases := []struct {
		name     string
		globs    []string
		wantErr  error
		wantText string
	}{
		{"no rules", nil, nil, ""},
		{"a star matches within one segment", []string{"database/migrations/*.sql"}, nil, ""},
		{
			"a star does not cross a slash",
			[]string{"database/*.sql"},
			ErrUnmatchedGlob,
			"database/*.sql",
		},
		{"a directory prefix matches below it", []string{"database/"}, nil, ""},
		{
			"an absent directory prefix matches nothing",
			[]string{"tests/"},
			ErrUnmatchedGlob,
			"tests/",
		},
		{"an exact path", []string{"main.go"}, nil, ""},
		{"a class", []string{"docs/spec/[a-c].md"}, nil, ""},
		{"a bad pattern", []string{"docs/[a"}, ErrBadGlob, "docs/[a"},
		{
			"one unmatched among matched ones",
			[]string{"main.go", "nothing.*"},
			ErrUnmatchedGlob,
			"nothing.*",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			built := []Rule{
				fakeRule{kind: "plain"},
				fakeRule{kind: "sql-fk", globs: testCase.globs},
			}

			err := Validate(built, tree)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Validate() err = %v, want %v", err, testCase.wantErr)
			}

			if err != nil && !strings.Contains(err.Error(), "rules[1] (sql-fk)") {
				t.Fatalf("Validate() err = %q, want the rule index and type", err)
			}

			if err != nil && !strings.Contains(err.Error(), testCase.wantText) {
				t.Fatalf("Validate() err = %q, want it to name %q", err, testCase.wantText)
			}
		})
	}
}

func BenchmarkValidate(b *testing.B) {
	paths := make([]string, 0, 10000)
	for idx := range 10000 {
		paths = append(paths, fmt.Sprintf("apps/mod%d/internal/file%d.go", idx%50, idx))
	}

	paths = append(paths, "database/migrations/001.sql")

	globs := make([]string, 0, 20)
	for idx := range 17 {
		globs = append(globs, fmt.Sprintf("apps/mod%d/internal/*.go", idx))
	}

	globs = append(globs, "database/migrations/*.sql", "*/migrations/", "*/mig*/")
	built := []Rule{fakeRule{kind: "bench", globs: globs}}

	b.ResetTimer()

	for range b.N {
		if err := Validate(built, paths); err != nil {
			b.Fatalf("Validate() = %v", err)
		}
	}

	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/1e6, "ms/validate")
}

func TestValidateDirectories(t *testing.T) {
	t.Parallel()

	tree := []string{
		"database/migrations/001.sql", "database/migrations/sub/002.sql",
		"docs/spec/a.md", "main.go",
	}

	testCases := []struct {
		name    string
		glob    string
		wantErr error
	}{
		{"a bad directory pattern", "[a/", ErrBadGlob},
		{"a wildcard directory matches below it", "database/m*/", nil},
		{"a nested wildcard directory", "database/*/sub/", nil},
		{"a wildcard directory is not a file", "docs/sp*/a.md/", ErrUnmatchedGlob},
		{"a wildcard directory is not the file itself", "main.g*/", ErrUnmatchedGlob},
		{"an escaped star is a literal name", `docs\*/`, ErrUnmatchedGlob},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			built := []Rule{fakeRule{kind: "sql-fk", globs: []string{testCase.glob}}}

			err := Validate(built, tree)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Validate() err = %v, want %v", err, testCase.wantErr)
			}

			if err != nil && !strings.Contains(err.Error(), fmt.Sprintf("%q", testCase.glob)) {
				t.Fatalf("Validate() err = %q, want it to name %q", err, testCase.glob)
			}
		})
	}
}

func TestValidateEscapedDirectory(t *testing.T) {
	t.Parallel()

	built := []Rule{fakeRule{kind: "sql-fk", globs: []string{`a\*/`}}}

	testCases := []struct {
		name    string
		tree    []string
		wantErr error
	}{
		{"the literal star directory", []string{"a*/x.go"}, nil},
		{"a sibling a star would have matched", []string{"ab/x.go"}, ErrUnmatchedGlob},
		{"the directory itself is no file under it", []string{"a*"}, ErrUnmatchedGlob},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if err := Validate(built, testCase.tree); !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Validate() err = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestValidateWildcardDirectory(t *testing.T) {
	t.Parallel()

	built := []Rule{fakeRule{kind: "sql-fk", globs: []string{"a/b*/"}}}

	testCases := []struct {
		name    string
		tree    []string
		wantErr error
	}{
		{"a file under a matching directory", []string{"a/bc/x.go"}, nil},
		{"a file deep under a matching directory", []string{"a/bc/d/e/x.go"}, nil},
		{"a file named like the directory", []string{"a/bc"}, ErrUnmatchedGlob},
		{"a sibling directory", []string{"a/c/x.go"}, ErrUnmatchedGlob},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if err := Validate(built, testCase.tree); !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Validate() err = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestDirectoryShortcutKeepsEveryResult(t *testing.T) {
	t.Parallel()

	files := []string{
		"a/x.go", "a/b/x.go", "a/bc/x.go", "a/bc/d/x.go", "a*/x.go", "ab/c/d/e.go",
		"x.go", "b/a/x.go", "a/b/c/d/e/f.go",
	}
	patterns := []string{
		"a/", "a/b*/", "*/", "*/*/", "a?/", "*/b*/d/", `a\*/`, "[a]/", "a/[bc]*/", "?/b/", "a/b/c/",
	}

	for _, pattern := range patterns {
		t.Run(pattern, func(t *testing.T) {
			t.Parallel()

			built := []Rule{fakeRule{kind: "probe", globs: []string{pattern}}}

			pending, err := collectGlobs(built)
			if err != nil {
				t.Fatalf("collectGlobs() = %v", err)
			}

			for _, file := range files {
				want := false

				for dir := path.Dir(file); dir != "."; dir = path.Dir(dir) {
					ok, _ := path.Match(strings.TrimSuffix(pattern, "/"), dir)
					want = want || ok
				}

				if got := pending[0].matches(file); got != want {
					t.Fatalf("matches(%q) = %v, want %v", file, got, want)
				}
			}
		})
	}
}

func TestCheckGlobs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		globs   []string
		wantErr error
	}{
		{"a glob needing a tree is not checked", []string{"nothing.*", "gone/"}, nil},
		{"a bad file pattern", []string{"main.go", "docs/[a"}, ErrBadGlob},
		{"a bad directory pattern", []string{"[a/"}, ErrBadGlob},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			checked := fakeRule{kind: "sql-fk", globs: testCase.globs}
			built := []Rule{fakeRule{kind: "plain"}, checked}

			err := CheckGlobs(built)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("CheckGlobs() err = %v, want %v", err, testCase.wantErr)
			}

			if err != nil && !strings.Contains(err.Error(), "rules[1] (sql-fk)") {
				t.Fatalf("CheckGlobs() err = %q, want the rule index and type", err)
			}
		})
	}
}
