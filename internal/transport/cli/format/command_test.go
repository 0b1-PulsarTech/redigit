package format

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/infra/format"
	"github.com/wrapped-owls/redigit/internal/transport/cli/write"
)

func TestRunRefusals(t *testing.T) {
	t.Parallel()

	row := `{"position":1,"blobs":[{"path":"a.go","mode":"100644","handle":"h1"}]}`
	missingTool := `{"format":{"languages":[{"name":"go","extensions":[".go"],"steps":[` +
		`{"cmd":["redigit-no-such-formatter"],"mode":"files","timeout":"30s"}]}]}}`

	testCases := []struct {
		name        string
		config      string
		extra       []string
		dropFirst   bool
		wantUsage   bool
		wantMissing bool
		wantText    string
	}{
		{
			name: "no format section", config: `{"write":{}}`,
			wantUsage: true, wantText: "no \"format\" section",
		},
		{
			name: "out equal to manifest", config: fakeConfig,
			extra:     []string{"--out", "{manifest}"},
			wantUsage: true, wantText: "--out must not be its input",
		},
		{
			name: "out blob dir equal to blob dir", config: fakeConfig,
			extra:     []string{"--out-blob-dir", "{blobs}"},
			wantUsage: true, wantText: "--out-blob-dir must not be its input",
		},
		{
			name: "a missing flag", config: fakeConfig, dropFirst: true,
			wantUsage: true, wantText: "--manifest is required",
		},
		{
			name: "an unknown flag", config: fakeConfig, extra: []string{"--nope"},
			wantUsage: true, wantText: "nope",
		},
		{
			name: "a positional argument", config: fakeConfig, extra: []string{"stray"},
			wantUsage: true, wantText: "unexpected argument",
		},
		{
			name: "a formatter that is not installed", config: missingTool,
			wantMissing: true, wantText: "is not installed",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fix := stage(t, []string{row}, map[string]string{"h1": "x"}, testCase.config)
			args := fix.args()

			if testCase.dropFirst {
				args = args[2:]
			}

			for _, extra := range testCase.extra {
				args = append(args, strings.NewReplacer(
					"{manifest}", fix.manifest, "{blobs}", fix.blobs,
				).Replace(extra))
			}

			var stdout bytes.Buffer

			err := Run(context.Background(), args, &stdout)
			if err == nil || errors.Is(err, write.ErrUsage) != testCase.wantUsage ||
				!strings.Contains(err.Error(), testCase.wantText) || stdout.Len() != 0 {
				t.Fatalf("Run() err = %v, stdout = %q", err, stdout.String())
			}

			var missing *format.MissingToolError
			if errors.As(err, &missing) != testCase.wantMissing {
				t.Fatalf("Run() err = %v, want missing tool = %v", err, testCase.wantMissing)
			}
		})
	}
}
