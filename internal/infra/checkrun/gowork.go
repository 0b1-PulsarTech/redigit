package checkrun

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
)

const goLanguage = "go"

func stepEnv(unit verifying.Unit, forced []string, fileFlags string) (run, keyed []string) {
	var flags string

	run, keyed, flags = inherited(unit.Language, forced)
	if unit.Language != goLanguage {
		return run, keyed
	}

	// The go command only reads GOFLAGS from its env file when the variable is empty.
	if flags == "" {
		flags = fileFlags
	}

	// Slots are not repositories, so VCS stamping would only fail or find the caller's repo.
	set := []string{"GO111MODULE=on", "GOFLAGS=" + strings.TrimSpace(flags+" -buildvcs=false")}

	if unit.Workspace == "" {
		set = append(set, "GOWORK=off")
	}

	return append(run, set...), append(keyed, set...)
}

func (runner *Runner) fileFlags(ctx context.Context, unit verifying.Unit) (string, error) {
	runner.mutex.Lock()
	defer runner.mutex.Unlock()

	return runner.lockedFileFlags(ctx, unit)
}

func (runner *Runner) lockedFileFlags(ctx context.Context, unit verifying.Unit) (string, error) {
	env, _, flags := inherited(goLanguage, nil)
	if unit.Language != goLanguage || flags != "" {
		return "", nil
	}

	if known, isKnown := runner.versions["goflags"]; isKnown {
		return known, nil
	}

	report, err := probe(
		ctx,
		stepRun{dir: string(filepath.Separator), env: env},
		[]string{"go", "env", "-json", "GOFLAGS"},
	)
	if err != nil {
		return "", err
	}

	var seen struct{ GOFLAGS string }
	if err = json.Unmarshal([]byte(report), &seen); err != nil {
		return "", fmt.Errorf("%w: go env GOFLAGS: %w", ErrVersion, err)
	}

	runner.versions["goflags"] = seen.GOFLAGS

	return seen.GOFLAGS, nil
}
