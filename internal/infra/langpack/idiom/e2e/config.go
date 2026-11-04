package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom/router"
)

var (
	ErrConfig   = errors.New("malformed go-e2e rule")
	ErrNoRouter = errors.New("go-e2e names no router")
	ErrNoDirs   = errors.New("go-e2e lists no dirs")
	ErrBadDir   = errors.New("go-e2e dir is a plain directory, not a pattern")
)

type Config struct {
	Router   json.RawMessage
	Dirs     []string
	Requests []string
}

type wire struct {
	Type     string          `json:"type"`
	Router   json.RawMessage `json:"router"`
	Dirs     []string        `json:"dirs"`
	Requests []string        `json:"requests"`
}

type routerWire struct {
	Handle []string `json:"handle"`
	Mount  []string `json:"mount"`
}

func decode(raw json.RawMessage) (Config, error) {
	var parsed wire

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&parsed); err != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrConfig, err)
	}

	if len(parsed.Router) == 0 {
		return Config{}, ErrNoRouter
	}

	if len(parsed.Dirs) == 0 {
		return Config{}, ErrNoDirs
	}

	for _, dir := range parsed.Dirs {
		if strings.ContainsAny(dir, `*?[\`) {
			return Config{}, fmt.Errorf("dir %q: %w", dir, ErrBadDir)
		}
	}

	for _, pattern := range parsed.Requests {
		if _, err := path.Match(pattern, ""); err != nil {
			return Config{}, fmt.Errorf("requests %q: %w: %w", pattern, router.ErrBadPattern, err)
		}
	}

	return Config{Router: parsed.Router, Dirs: parsed.Dirs, Requests: parsed.Requests}, nil
}

func routerPatterns(raw json.RawMessage) (handle, mount []string, err error) {
	var parsed routerWire
	if err = json.Unmarshal(raw, &parsed); err != nil {
		return nil, nil, fmt.Errorf("%w: router: %w", ErrConfig, err)
	}

	return parsed.Handle, parsed.Mount, nil
}
