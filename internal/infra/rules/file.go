package rules

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
)

var (
	ErrMalformed   = errors.New("malformed config file")
	ErrMissingType = errors.New("missing type")
	ErrEmpty       = errors.New("empty file")
)

type (
	File struct {
		// Language sections stay raw: the one table that knows which settings
		// exist also owns decoding them, so no second list can drift from it.
		Languages map[string]map[string]json.RawMessage
		Rules     []Spec

		// Sections below belong to later stages, which decode their own
		// shape; keeping them raw means this package never has to know it.
		Slice   json.RawMessage
		Passes  json.RawMessage
		Format  json.RawMessage
		Write   json.RawMessage
		Verify  json.RawMessage
		Lineage json.RawMessage
		Lint    json.RawMessage
	}

	Spec struct {
		Type string
		Raw  json.RawMessage
	}

	decodeError struct{ err error }
)

func (failure decodeError) Error() string { return failure.err.Error() }

func (failure decodeError) Unwrap() []error { return []error{ErrMalformed, failure.err} }

func malformed(format string, args ...any) error {
	return decodeError{fmt.Errorf(format, args...)}
}

func (file *File) keepRaw(key string, raw json.RawMessage) bool {
	slots := map[string]*json.RawMessage{
		"slice": &file.Slice, "passes": &file.Passes, "format": &file.Format,
		"write": &file.Write, "verify": &file.Verify, "lineage": &file.Lineage,
		"lint": &file.Lint,
	}

	slot, found := slots[key]
	if found {
		*slot = raw
	}

	return found || key == "languages" || key == "rules"
}

func Decode(content []byte) (File, error) {
	top, err := topLevel(content)
	if err != nil {
		return File{}, err
	}

	var file File

	for _, key := range slices.Sorted(maps.Keys(top)) {
		if !file.keepRaw(key, top[key]) {
			return File{}, malformed("unknown field %q", key)
		}
	}

	if raw, found := top["languages"]; found {
		if file.Languages, err = decodeLanguages(raw); err != nil {
			return File{}, err
		}
	}

	if raw, found := top["rules"]; found {
		if file.Rules, err = decodeRules(raw); err != nil {
			return File{}, err
		}
	}

	return file, nil
}
