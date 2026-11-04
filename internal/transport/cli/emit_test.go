package cli

import (
	"bytes"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/emitting"
)

func TestDispatchEmitWritesTheManifestToStdout(t *testing.T) {
	t.Parallel()

	live := settledLive(t)
	live.EXPECT().
		Emit(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]emitting.Commit{{Position: 3, Subject: "DRAFT: feat: a"}}, nil)

	counter := &startCounter{live: live}

	var stdout, stderr bytes.Buffer

	args := []string{cmdEmit, "--repo", ".", "--blob-dir", t.TempDir()}

	env := runEnv{Start: counter.start}

	if err := dispatch(t.Context(), args, &stdout, &stderr, env); err != nil {
		t.Fatalf("dispatch() err = %v", err)
	}

	if !strings.Contains(stdout.String(), `"subject":"DRAFT: feat: a"`) || stderr.Len() != 0 {
		t.Fatalf("stdout = %q, stderr = %q, want stdout alone", stdout.String(), stderr.String())
	}
}

func TestWriteManifestKeepsEveryCommitField(t *testing.T) {
	t.Parallel()

	key := entities.DeclKey{File: "a.go", Kind: "func", Name: "F"}
	target := entities.DeclKey{File: "b.go", Kind: "type", Name: "T"}

	testCases := []struct {
		name    string
		commits []emitting.Commit
		want    string
	}{
		{"no commit writes nothing", nil, ""},
		{
			"a commit with every list filled",
			[]emitting.Commit{{
				Position: 2, Subject: "s", Shape: "feature", Feature: "f", Reason: "r", IsWIP: true,
				Chunks: []emitting.Chunk{
					{Key: key, Span: entities.Span{Start: 3, End: 8}, Unit: "u"},
				},
				Blobs: []emitting.Blob{{Path: "a.go", Mode: "100644", Handle: "h"}},
				Risks: []emitting.Risk{{Chunk: key, Target: target, BornAt: 5}},
			}},
			`{"position":2,"subject":"s","shape":"feature","feature":"f","reason":"r","is_wip":true,` +
				`"chunks":[{"key":"a.go:func:F","span":[3,8],"unit":"u"}],` +
				`"blobs":[{"path":"a.go","mode":"100644","handle":"h"}],` +
				`"risks":[{"chunk":"a.go:func:F","target":"b.go:type:T","born_at":5}]}` + "\n",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			if err := writeManifest(&out, testCase.commits); err != nil {
				t.Fatalf("writeManifest() err = %v", err)
			}

			if out.String() != testCase.want {
				t.Fatalf("writeManifest() = %q, want %q", out.String(), testCase.want)
			}
		})
	}
}
