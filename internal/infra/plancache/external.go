package plancache

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func mixExternal(hasher hash.Hash, settings map[string]json.RawMessage) error {
	// Files go env names only by path: a changed byte must change the key.
	if err := refuseFileFlags(settings); err != nil {
		return err
	}

	if err := mixDriver(hasher); err != nil {
		return err
	}

	// A workspace's use dirs are sources the key never sees, so a user's own is refused.
	work, err := settingString(settings, "GOWORK")
	if err != nil {
		return err
	}

	if work != "" && work != "off" {
		return fmt.Errorf("%w: GOWORK names a workspace the key cannot hash", ErrToolchain)
	}

	return nil
}

func mixDriver(hasher hash.Hash) error {
	// go/packages runs a gopackagesdriver found on PATH even when the variable is empty.
	value := os.Getenv("GOPACKAGESDRIVER")
	if value == "off" {
		return nil
	}

	name := value
	if name == "" {
		name = "gopackagesdriver"
	}

	// The loader runs it from the repo dir, so a relative path would mean another file here.
	if strings.ContainsRune(name, filepath.Separator) && !filepath.IsAbs(name) {
		return fmt.Errorf("%w: GOPACKAGESDRIVER is a relative path", ErrToolchain)
	}

	path, err := exec.LookPath(name)
	if err != nil {
		if value == "" && errors.Is(err, exec.ErrNotFound) {
			return nil
		}

		return fmt.Errorf("%w: find GOPACKAGESDRIVER: %w", ErrToolchain, err)
	}

	digest, err := FileDigest(path)
	if err != nil {
		return fmt.Errorf("%w: GOPACKAGESDRIVER: %w", ErrToolchain, err)
	}

	put(hasher, "driver-bytes", digest)

	return nil
}

func refuseFileFlags(settings map[string]json.RawMessage) error {
	flags, err := settingString(settings, "GOFLAGS")
	if err != nil {
		return err
	}

	// cmd/go honours quotes in GOFLAGS, so no tokenising: the name anywhere is refused.
	for _, name := range []string{"modfile", "overlay"} {
		if strings.Contains(flags, name) {
			return fmt.Errorf(
				"%w: GOFLAGS carries %s, a file the key cannot hash",
				ErrToolchain,
				name,
			)
		}
	}

	return nil
}

func settingString(settings map[string]json.RawMessage, key string) (string, error) {
	raw, ok := settings[key]
	if !ok {
		return "", nil
	}

	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("%w: decode %s: %w", ErrToolchain, key, err)
	}

	return value, nil
}
