package chunk

import (
	"context"
	"strings"
	"testing"
)

func TestChangelogChunks(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		source string
		names  []string
	}{
		{
			name:   "the preamble rides with the first entry",
			source: "# Changelog\n\nintro\n\n## one\n\nbody\n\n## two\n\nbody\n",
			names:  []string{"<file>", "<file>#2"},
		},
		{
			name:   "a heading of level three opens no entry",
			source: "# C\n\n## one\n\n### Changed\n\n## two\n",
			names:  []string{"<file>", "<file>#2"},
		},
		{
			name:   "one entry stays whole",
			source: "# C\n\n## one\n",
			names:  []string{"<file>"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := Changelog{}.Chunks(
				context.Background(),
				"docs/CHANGELOG.md",
				[]byte(testCase.source),
			)
			if err != nil {
				t.Fatalf("Chunks() error = %v", err)
			}

			var names []string

			rebuilt := strings.Builder{}

			for _, decl := range got {
				names = append(names, decl.Key.Name)
				rebuilt.WriteString(testCase.source[decl.Span.Start:decl.Span.End])
			}

			if strings.Join(names, ",") != strings.Join(testCase.names, ",") {
				t.Fatalf("names = %v, want %v", names, testCase.names)
			}

			if rebuilt.String() != testCase.source {
				t.Fatalf("the pieces do not tile the file: %q", rebuilt.String())
			}
		})
	}
}

func TestChangelogHandles(t *testing.T) {
	t.Parallel()

	if !(Changelog{}).Handles("docs/CHANGELOG.md") || (Changelog{}).Handles("docs/README.md") {
		t.Fatal("Handles() must claim CHANGELOG.md and nothing else")
	}
}
