package verify

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/wrapped-owls/redigit/internal/infra/checkrun"
	"github.com/wrapped-owls/redigit/internal/infra/rules"
)

func decodeAs(raw json.RawMessage, target any, kind string) error {
	if err := rules.DecodeStrict(raw, target, kind); err != nil {
		return err //nolint:wrapcheck // the caller names the file, section and key
	}

	return nil
}

func decodeMembers(raw json.RawMessage, handlers members) error {
	var found map[string]json.RawMessage
	if err := decodeAs(raw, &found, "an object"); err != nil {
		return err
	}

	for _, key := range slices.Sorted(maps.Keys(found)) {
		handle, isKnown := handlers[key]
		if !isKnown {
			return fmt.Errorf("unknown key %q", key)
		}

		if err := handle(found[key]); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
	}

	return nil
}

func decodeList(raw json.RawMessage, target *[]string) error {
	return decodeAs(raw, target, "a list of strings")
}

func decodeCount(raw json.RawMessage, target *int) error {
	if err := decodeAs(raw, target, "a whole number"); err != nil {
		return err
	}

	if *target < 1 {
		return errors.New("must be at least 1")
	}

	return nil
}

func decodeParallel(raw json.RawMessage, target *int) error {
	if err := decodeCount(raw, target); err != nil {
		return err
	}

	if *target > runtime.NumCPU() {
		return fmt.Errorf("must be at most the %d cpus", runtime.NumCPU())
	}

	return nil
}

func decodeLanguages(raw json.RawMessage, target *settings) error {
	var named map[string]json.RawMessage
	if err := decodeAs(raw, &named, "an object"); err != nil {
		return err
	}

	target.languages = map[string]checkrun.Language{}

	for _, name := range slices.Sorted(maps.Keys(named)) {
		if err := decodeLanguage(name, named[name], target); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}

	return nil
}

func decodeLanguage(name string, raw json.RawMessage, target *settings) error {
	var marker string
	if name == goName {
		marker = goMarker
	}

	var language checkrun.Language

	err := decodeMembers(raw, members{
		"marker":  func(raw json.RawMessage) error { return decodeAs(raw, &marker, "a string") },
		"version": func(raw json.RawMessage) error { return decodeList(raw, &language.Version) },
		"env":     func(raw json.RawMessage) error { return decodeList(raw, &language.Env) },
		"steps":   func(raw json.RawMessage) error { return decodeSteps(raw, &language.Steps) },
	})
	if err != nil {
		return err
	}

	owner, isTaken := target.markers[marker]

	switch {
	case marker == "" || strings.Contains(marker, "/"):
		return errors.New("marker must be a bare file name")
	case len(language.Steps) == 0:
		return errors.New("steps must hold at least one step")
	case isTaken && owner != name:
		return fmt.Errorf("marker %q already belongs to %q", marker, owner)
	}

	target.markers[marker] = name
	target.languages[name] = language

	return nil
}

func decodeSteps(raw json.RawMessage, target *[]checkrun.Step) error {
	var items []json.RawMessage
	if err := decodeAs(raw, &items, "a list of steps"); err != nil {
		return err
	}

	for _, item := range items {
		var step checkrun.Step

		err := decodeMembers(item, members{
			"cmd":      func(raw json.RawMessage) error { return decodeList(raw, &step.Argv) },
			"tolerate": func(raw json.RawMessage) error { return decodeList(raw, &step.Tolerate) },
			"timeout":  func(raw json.RawMessage) error { return decodeTimeout(raw, &step.Timeout) },
		})
		if err != nil {
			return err
		}

		if len(step.Argv) == 0 {
			return errors.New("a step needs a cmd")
		}

		*target = append(*target, step)
	}

	return nil
}

func decodeTimeout(raw json.RawMessage, target *time.Duration) error {
	var text string
	if err := decodeAs(raw, &text, "a duration string"); err != nil {
		return err
	}

	parsed, err := time.ParseDuration(text)
	if err != nil || parsed <= 0 {
		return errors.New("must be a positive duration such as \"5m\"")
	}

	*target = parsed

	return nil
}
