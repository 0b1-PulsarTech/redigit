package format

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type configFile struct {
	name    string
	content []byte
}

func readConfigs(root string, names []string) ([]configFile, error) {
	if len(names) == 0 {
		return nil, nil
	}

	for _, name := range names {
		if !filepath.IsLocal(name) {
			return nil, fmt.Errorf(
				"%w: copy_files entry %q leaves the config root",
				ErrConfig,
				name,
			)
		}
	}

	if root == "" {
		root = "."
	}

	handle, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open config root: %w", err)
	}

	defer func() { _ = handle.Close() }()

	configs := make([]configFile, 0, len(names))

	for _, name := range names {
		// the root refuses a symlink that points out of config_root
		content, readErr := handle.ReadFile(name)
		if errors.Is(readErr, os.ErrNotExist) {
			continue
		}

		if readErr != nil {
			return nil, fmt.Errorf("read config file %s: %w", name, readErr)
		}

		configs = append(configs, configFile{name: name, content: content})
	}

	return configs, nil
}

func (engine *Engine) copyConfig(root *os.Root) error {
	for _, config := range engine.configs {
		if err := writeUnder(root, config.name, config.content); err != nil {
			return err
		}
	}

	return nil
}

func writeUnder(root *os.Root, name string, content []byte) error {
	const (
		dirMode  = 0o750
		fileMode = 0o600
	)

	if dir := filepath.Dir(name); dir != "." {
		if err := root.MkdirAll(dir, dirMode); err != nil {
			return fmt.Errorf("make dir for %s: %w", name, err)
		}
	}

	if err := root.WriteFile(name, content, fileMode); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}

	return nil
}

func unstarted(err error) run {
	// a work dir that cannot be set up fails whatever blobs share the chunk, like a missing binary
	return run{err: fmt.Errorf("%w: %w", errStart, err)}
}

func readCapped(root *os.Root, name string, limit int) ([]byte, error) {
	// A fifo left by the tool opens without blocking and the kind is judged on the handle
	handle, err := root.OpenFile(name, readFlags, 0)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}

	defer func() { _ = handle.Close() }()

	entry, err := handle.Stat()
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}

	if !entry.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s is not a regular file", ErrOutputKind, name)
	}

	content, err := io.ReadAll(io.LimitReader(handle, int64(limit)+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}

	if len(content) > limit {
		return nil, fmt.Errorf("%w: %s", ErrOutputTooLarge, name)
	}

	return content, nil
}
