package checkrun

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
)

func (runner *Runner) Identity(ctx context.Context, unit verifying.Unit) (string, error) {
	language, err := runner.language(unit.Language)
	if err != nil {
		return "", err
	}

	version, err := runner.version(ctx, unit, language)
	if err != nil {
		return "", err
	}

	parts := []string{unit.Language, version}

	for _, step := range language.Steps {
		argv, tolerated := strings.Join(step.Argv, "\x00"), strings.Join(step.Tolerate, "\x1d")
		parts = append(parts, argv+"\x1e"+tolerated)
	}

	fileFlags, err := runner.fileFlags(ctx, unit)
	if err != nil {
		return "", err
	}

	if unit.Language == goLanguage {
		toolchain, envErr := runner.goEnv(ctx, unit)
		if envErr != nil {
			return "", envErr
		}

		parts = append(parts, "workspace="+unit.Workspace, "goenv="+toolchain)
	}

	_, keyed := stepEnv(unit, language.Env, fileFlags)

	for _, entry := range keyed {
		parts = append(parts, "env:"+entry)
	}

	return strings.Join(parts, "\x1f"), nil
}

func (runner *Runner) version(
	ctx context.Context,
	unit verifying.Unit,
	language Language,
) (string, error) {
	if len(language.Version) == 0 {
		return "", nil
	}

	return runner.probed(ctx, unit, "version", language.Version)
}

func (runner *Runner) goEnv(ctx context.Context, unit verifying.Unit) (string, error) {
	const goEnvNames = "GOFLAGS CGO_ENABLED GOOS GOARCH GOEXPERIMENT GOTOOLCHAIN GOAMD64 GOARM GOARM64 " +
		"GO386 GOMIPS GOPPC64 GORISCV64 CC CXX CGO_CFLAGS CGO_CPPFLAGS CGO_CXXFLAGS CGO_LDFLAGS " +
		"GOVERSION GO111MODULE GOWASM GOMIPS64 GOFIPS140 GODEBUG GOCACHEPROG"

	argv := append([]string{"go", "env", "-json", "GOMOD", "GOWORK"}, strings.Fields(goEnvNames)...)

	report, err := runner.probed(ctx, unit, "env", argv)
	if err != nil {
		return "", err
	}

	var seen struct{ GOMOD, GOWORK, GOCACHEPROG string }
	if err = json.Unmarshal([]byte(report), &seen); err != nil {
		return "", fmt.Errorf("%w: go env: %w", ErrVersion, err)
	}

	// An empty GOCACHEPROG in the step env would not override the go env file's, so refuse.
	if seen.GOCACHEPROG != "" {
		return "", fmt.Errorf("%w: GOCACHEPROG=%q", ErrCacheProg, seen.GOCACHEPROG)
	}

	if (seen.GOMOD != "" && seen.GOMOD != os.DevNull) ||
		(seen.GOWORK != "" && seen.GOWORK != "off") {
		return "", fmt.Errorf("%w: GOMOD=%q GOWORK=%q", ErrAmbient, seen.GOMOD, seen.GOWORK)
	}

	return report, nil
}

func (runner *Runner) probed(
	ctx context.Context,
	unit verifying.Unit,
	kind string,
	argv []string,
) (string, error) {
	isOff := strconv.FormatBool(unit.Workspace == "")
	key := strings.Join([]string{unit.Language, kind, isOff}, "\x00")

	runner.mutex.Lock()
	defer runner.mutex.Unlock()

	if known, isKnown := runner.versions[key]; isKnown {
		return known, nil
	}

	fileFlags, err := runner.lockedFileFlags(ctx, unit)
	if err != nil {
		return "", err
	}

	// The filesystem root has no go.mod or go.work above it for a probe to pick up.
	env, _ := stepEnv(unit, nil, fileFlags)

	output, err := probe(ctx, stepRun{dir: string(filepath.Separator), env: env}, argv)
	if err != nil {
		return "", err
	}

	runner.versions[key] = output

	return output, nil
}
