package subjects

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/wrapped-owls/redigit/internal/transport/cli/write"
)

const (
	rowZero = `{"position":0,"subject":"DRAFT: feat(a): x","shape":"feature","is_wip":false,` +
		`"chunks":[{"key":"k1","span":[0,9],"unit":"u"},{"key":"k2","span":[9,19],"unit":"u"}],` +
		`"blobs":[{"path":"a.go","mode":"100644","handle":"h"}],"risks":[]}` + "\n"
	rowOne = `{"position":1,"subject":"DRAFT: feat(b): y","shape":"layer","is_wip":true,` +
		`"chunks":[{"key":"k3","span":[0,5],"unit":"v"}],` +
		`"blobs":[{"path":"b.go","mode":"100644","handle":"h"}],` +
		`"risks":[{"chunk":"k3","target":"t","born_at":4}]}` + "\n"
	goodZero = ":bug: fix(a): Hand written"
	goodOne  = ":gem: feat(b): Other hand"
)

var errWriterBroken = errors.New("writer broken")

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errWriterBroken }

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}

	return path
}

func TestRunIsAUsageErrorOnMisuse(t *testing.T) {
	t.Parallel()

	manifestPath := writeTemp(t, "m.jsonl", rowZero)

	testCases := []struct {
		name string
		args []string
	}{
		{name: "no arguments"},
		{name: "unknown subcommand", args: []string{"nope"}},
		{name: "flag as subcommand", args: []string{"--repo", "."}},
		{name: "unknown flag", args: []string{"digest", "--nope"}},
		{name: "stray argument", args: []string{"digest", "--manifest", manifestPath, "extra"}},
		{name: "digest without manifest", args: []string{"digest"}},
		{name: "check without subjects", args: []string{"check", "--manifest", manifestPath}},
		{name: "check without manifest", args: []string{"check", "--subjects", manifestPath}},
		{name: "carry without out", args: []string{
			"carry", "--from", manifestPath, "--subjects", manifestPath, "--manifest", manifestPath,
		}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var stdout bytes.Buffer

			err := Run(context.Background(), testCase.args, &stdout)
			if !errors.Is(err, write.ErrUsage) || stdout.Len() != 0 {
				t.Fatalf(
					"Run() err = %v, stdout = %q, want a usage error alone",
					err,
					stdout.String(),
				)
			}
		})
	}
}
