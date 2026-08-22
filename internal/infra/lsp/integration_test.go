//go:build integration

package lsp

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// This is the proof the PLANO.md blind spot is closed: a file behind
// //go:build integration is invisible to textDocument/references unless
// BuildFlags carries the matching -tags flag, silently and without error.
// It drives a real gopls process against a throwaway module, never a fake.
func TestSession_References_RequiresBuildTag(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFixtureModule(t, dir)

	// "func Target() {}" on the third line (0-based line 2); "Target"
	// starts right after "func ".
	at := entities.Location{File: filepath.Join(dir, "normal.go"), Line: 2, Char: 5}

	testCases := []struct {
		name       string
		buildFlags []string
		wantTagged bool
	}{
		{
			name:       "with build tag sees the tagged call site",
			buildFlags: []string{"-tags=integration"},
			wantTagged: true,
		},
		{name: "without build tag misses the tagged call site", buildFlags: nil, wantTagged: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// Each subtest gets its own context: sharing one built in the
			// parent with a deferred cancel would cancel it the instant the
			// parent's body returns, which happens before parallel subtests
			// actually run their bodies.
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			session, err := StartSession(ctx, ServerConfig{
				Command:    "gopls",
				Args:       []string{"-mode=stdio"},
				RootPath:   dir,
				BuildFlags: testCase.buildFlags,
			})
			if err != nil {
				t.Fatalf("StartSession: %v", err)
			}
			defer func() {
				if err := session.Close(ctx); err != nil {
					t.Logf("Close: %v", err)
				}
			}()

			refs, err := session.References(ctx, at)
			if err != nil {
				t.Fatalf("References: %v", err)
			}

			gotTagged := slices.ContainsFunc(refs, func(loc entities.Location) bool {
				return filepath.Base(loc.File) == "tagged.go"
			})
			if gotTagged != testCase.wantTagged {
				t.Fatalf(
					"references saw tagged.go = %v, want %v (refs=%+v)",
					gotTagged,
					testCase.wantTagged,
					refs,
				)
			}
		})
	}
}

// writeFixtureModule creates a minimal Go module under dir with a plain
// declaration and a build-tagged caller of it, the exact shape the hard
// facts describe: a normal file's declaration, referenced only from a file
// behind //go:build integration.
func writeFixtureModule(t testing.TB, dir string) {
	t.Helper()

	files := map[string]string{
		"go.mod":    "module example.com/fixture\n\ngo 1.26\n",
		"normal.go": "package fixture\n\nfunc Target() {}\n",
		"tagged.go": "//go:build integration\n\npackage fixture\n\nfunc CallSite() {\n\tTarget()\n}\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}
