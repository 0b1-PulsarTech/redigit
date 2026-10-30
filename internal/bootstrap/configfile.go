package bootstrap

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/rules"
)

const (
	goLanguage     = "go"
	dartLanguage   = "dart"
	webLanguage    = "web"
	configLanguage = "config"
	opaqueName     = "opaque"
)

var (
	ErrUnknownLanguage = errors.New("unknown language")
	ErrUnknownSetting  = errors.New("unknown setting")
)

type ConfigFile struct {
	Path  string
	File  rules.File
	Rules []rules.Rule
	Bytes []byte
}

func compiledRegistry() rules.Registry {
	return rules.NewRegistry(map[string]rules.Constructor{})
}

func LoadConfigFile(path string) (ConfigFile, error) {
	return loadConfigFile(path, compiledRegistry())
}

func loadConfigFile(path string, registry rules.Registry) (ConfigFile, error) {
	content, err := rules.ReadBounded(path)
	if err != nil {
		return ConfigFile{}, fileError(path, err)
	}

	file, err := rules.Decode(content)
	if err != nil {
		return ConfigFile{}, fileError(path, err)
	}

	built, err := registry.Build(file.Rules)
	if err != nil {
		return ConfigFile{}, fileError(path, err)
	}

	if err = checkLanguages(file.Languages); err != nil {
		return ConfigFile{}, fileError(path, err)
	}

	// Syntax needs no tree, so a malformed glob fails here rather than in Start.
	if err = rules.CheckGlobs(built); err != nil {
		return ConfigFile{}, fileError(path, err)
	}

	return ConfigFile{Path: path, File: file, Rules: built, Bytes: content}, nil
}

func checkLanguages(languages map[string]map[string]json.RawMessage) error {
	known := map[string]map[string]setting{}

	for _, entry := range settings() {
		if known[entry.language] == nil {
			known[entry.language] = map[string]setting{}
		}

		known[entry.language][entry.key] = entry
	}

	var scratch Config

	for _, name := range slices.Sorted(maps.Keys(languages)) {
		rows, found := known[name]
		if !found {
			return fmt.Errorf(
				"%w %q (settings exist for: %s)",
				ErrUnknownLanguage, name, strings.Join(slices.Sorted(maps.Keys(known)), ", "),
			)
		}

		for _, key := range slices.Sorted(maps.Keys(languages[name])) {
			entry, listed := rows[key]
			if !listed {
				return fmt.Errorf(
					"language %q: %w %q (known: %s)",
					name, ErrUnknownSetting, key,
					strings.Join(slices.Sorted(maps.Keys(rows)), ", "),
				)
			}

			// Decoding here, not only in Apply, makes a bad value fail at load.
			if err := entry.apply(languages[name][key], &scratch); err != nil {
				return settingError(entry, err)
			}
		}
	}

	return nil
}

type configError struct {
	path string
	err  error
}

func fileError(path string, err error) error {
	return configError{path: path, err: err}
}

func (failure configError) Error() string {
	return "config file " + failure.path + ": " + failure.err.Error()
}

func (failure configError) Unwrap() []error {
	// Both sentinels stay matchable while only the cause is printed.
	return []error{ErrConfig, failure.err}
}

func validateRules(cfg Config, snap entities.Snapshot) error {
	if err := rules.Validate(cfg.Rules, snap.Files); err != nil {
		return fileError(cfg.ConfigPath, err)
	}

	return nil
}

func ApplyConfigFile(cfg Config, file ConfigFile, explicit func(string) bool) (Config, error) {
	for _, entry := range settings() {
		raw, found := file.File.Languages[entry.language][entry.key]

		// A flag replaces the file's whole setting for it; the two never merge.
		if !found || explicit(entry.flag) {
			continue
		}

		if err := entry.apply(raw, &cfg); err != nil {
			return cfg, fileError(file.Path, settingError(entry, err))
		}
	}

	cfg.Rules = file.Rules
	cfg.ConfigPath = file.Path
	cfg.ConfigBytes = file.Bytes
	cfg.Sections = file.File

	return cfg, nil
}

func settingError(entry setting, err error) error {
	return fmt.Errorf("languages.%s.%s: %w", entry.language, entry.key, err)
}
