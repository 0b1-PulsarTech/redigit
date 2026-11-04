package router

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
)

var (
	ErrConfig           = errors.New("malformed go-router rule")
	ErrNoHandle         = errors.New("handle names no route call")
	ErrBadPattern       = errors.New("malformed pattern")
	ErrDuplicatePattern = errors.New("pattern listed twice")
)

type Config struct {
	Handle       []string
	Mount        []string
	GroupMethods bool
	Paths        []string
}

type wire struct {
	Type         string   `json:"type"`
	Handle       []string `json:"handle"`
	Mount        []string `json:"mount"`
	GroupMethods bool     `json:"groupMethods"`
	Paths        []string `json:"paths"`
}

func decode(raw json.RawMessage) (Config, error) {
	var parsed wire

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&parsed); err != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrConfig, err)
	}

	if len(parsed.Handle) == 0 {
		return Config{}, ErrNoHandle
	}

	for _, field := range []struct {
		name     string
		patterns []string
	}{
		{"handle", parsed.Handle},
		{"mount", parsed.Mount},
		{"paths", parsed.Paths},
	} {
		if err := checkPatterns(field.name, field.patterns); err != nil {
			return Config{}, err
		}
	}

	return Config{
		Handle:       parsed.Handle,
		Mount:        parsed.Mount,
		GroupMethods: parsed.GroupMethods,
		Paths:        parsed.Paths,
	}, nil
}

func checkPatterns(field string, patterns []string) error {
	seen := map[string]bool{}

	for _, pattern := range patterns {
		if _, err := path.Match(strings.TrimSuffix(pattern, "/"), ""); err != nil {
			return fmt.Errorf("%s %q: %w: %w", field, pattern, ErrBadPattern, err)
		}

		if seen[pattern] {
			return fmt.Errorf("%s %q: %w", field, pattern, ErrDuplicatePattern)
		}

		seen[pattern] = true
	}

	return nil
}

func (cfg Config) within(file string) bool {
	if len(cfg.Paths) == 0 {
		return true
	}

	for _, glob := range cfg.Paths {
		if !strings.HasSuffix(glob, "/") {
			if ok, _ := path.Match(glob, file); ok {
				return true
			}

			continue
		}

		// A trailing slash means any file under a directory the pattern matches.
		for dir := path.Dir(file); dir != "." && dir != "/"; dir = path.Dir(dir) {
			if ok, _ := path.Match(strings.TrimSuffix(glob, "/"), dir); ok {
				return true
			}
		}
	}

	return false
}
