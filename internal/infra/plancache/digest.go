package plancache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
)

func FileDigest(path string) (string, error) {
	// #nosec G304 G703 -- the cache hashes the files the operator pointed the run at.
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open for digest: %w", err)
	}

	defer func() { _ = file.Close() }()

	hasher := sha256.New()
	if _, err = io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("read for digest: %w", err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func ExecutableDigest() (string, error) {
	// By bytes: a version string would not see a rebuild that plans differently.
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("find own executable: %w", err)
	}

	return FileDigest(path)
}

func CommandDigest(command string) (string, error) {
	path, err := exec.LookPath(command)
	if err != nil {
		return "", fmt.Errorf("find language server: %w", err)
	}

	return FileDigest(path)
}

func BytesDigest(content []byte) string {
	sum := sha256.Sum256(content)

	return hex.EncodeToString(sum[:])
}
