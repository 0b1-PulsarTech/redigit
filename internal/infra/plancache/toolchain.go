package plancache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

var ErrToolchain = errors.New("cannot identify the go toolchain")

func GoToolchain(ctx context.Context, repo, commit string) (string, error) {
	// The commit's own go.mod picks the toolchain, so ask from a dir that holds it.
	dir, err := moduleProbeDir(ctx, repo, commit)
	if err != nil {
		return "", err
	}

	defer func() { _ = os.RemoveAll(dir) }()

	// The whole configuration the loader runs under.
	environment, err := runGo(ctx, dir, "env", "-json")
	if err != nil {
		return "", err
	}

	version, err := runGo(ctx, dir, "version")
	if err != nil {
		return "", err
	}

	var settings map[string]json.RawMessage

	var goroot string

	if err = json.Unmarshal(environment, &settings); err != nil ||
		json.Unmarshal(settings["GOROOT"], &goroot) != nil || goroot == "" {
		return "", fmt.Errorf("%w: no GOROOT in go env", ErrToolchain)
	}

	dropProbePaths(settings, dir)

	stable, err := json.Marshal(settings)
	if err != nil {
		return "", fmt.Errorf("%w: encode go env: %w", ErrToolchain, err)
	}

	// The binary GOROOT names, not the go on PATH: GOTOOLCHAIN=auto can switch.
	compiler, err := exec.LookPath(filepath.Join(goroot, "bin", "go"))
	if err != nil {
		return "", fmt.Errorf("%w: find the toolchain go: %w", ErrToolchain, err)
	}

	binary, err := FileDigest(compiler)
	if err != nil {
		return "", err
	}

	hasher := sha256.New()

	// go env may not show these two, and both change what the loader sees.
	put(hasher, "env", string(stable))
	put(hasher, "version", string(version))
	put(hasher, "binary", binary)
	put(hasher, "GOPACKAGESDRIVER", os.Getenv("GOPACKAGESDRIVER"))

	if err = mixExternal(hasher, settings); err != nil {
		return "", err
	}

	put(hasher, "GODEBUG", os.Getenv("GODEBUG"))

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func runGo(ctx context.Context, dir string, args ...string) ([]byte, error) {
	var stdout, stderr bytes.Buffer

	// #nosec G204 -- the arguments are literals from GoToolchain.
	command := exec.CommandContext(ctx, "go", args...)
	command.Dir = dir
	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Run(); err != nil {
		if why := excerpt(stderr.Bytes()); why != "" {
			return nil, fmt.Errorf("%w: run go: %w: %s", ErrToolchain, err, why)
		}

		return nil, fmt.Errorf("%w: run go: %w", ErrToolchain, err)
	}

	return stdout.Bytes(), nil
}

func excerpt(stderr []byte) string {
	// A toolchain download or a bad go.mod says why only on stderr.
	const limit = 240

	text := strings.TrimSpace(string(stderr))
	if len(text) <= limit {
		return text
	}

	cut := limit
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}

	return text[:cut]
}

func dropProbePaths(settings map[string]json.RawMessage, dir string) {
	// Only the probe dir's own paths vary per run; a user's GOWORK=off stays in the key.
	delete(settings, "GOGCCFLAGS")

	roots := []string{dir}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		roots = append(roots, resolved)
	}

	for name, file := range map[string]string{"GOMOD": goModFile, "GOWORK": goWorkFile} {
		var value string
		if json.Unmarshal(settings[name], &value) != nil {
			continue
		}

		isProbe := name == "GOMOD" && value == os.DevNull

		for _, root := range roots {
			isProbe = isProbe || value == filepath.Join(root, file)
		}

		if isProbe {
			delete(settings, name)
		}
	}
}
