package sliceconf

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
)

var ErrInvalid = errors.New("invalid slice section")

type Config struct {
	Functions []string `json:"functions"`
	Errcheck  bool     `json:"errcheck"`
}

func Decode(raw json.RawMessage) (Config, error) {
	if len(raw) == 0 {
		return Config{}, nil
	}

	var found Config

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	// A null decodes into a struct without error, and it is refused here.
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return Config{}, fmt.Errorf("%w: expected an object", ErrInvalid)
	}

	if err := decoder.Decode(&found); err != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}

	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("%w: trailing data", ErrInvalid)
	}

	for _, glob := range found.Functions {
		if _, err := path.Match(glob, ""); glob == "" || err != nil {
			return Config{}, fmt.Errorf("%w: functions: bad glob %q", ErrInvalid, glob)
		}
	}

	return found, nil
}
