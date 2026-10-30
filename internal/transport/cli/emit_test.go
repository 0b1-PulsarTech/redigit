package cli

import (
	"bytes"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

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
