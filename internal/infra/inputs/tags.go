package inputs

import (
	"errors"
	"strings"
	"unicode/utf8"
)

var (
	ErrVirtual          = errors.New("expected ID to PATH pairs, both non-empty")
	ErrDuplicateVirtual = errors.New("virtual ID listed twice")
	ErrNotUTF8          = errors.New("text holds U+FFFD or invalid UTF-8")
)

func AddVirtual(virtual map[string]string, id, path string) error {
	// The one rule for a virtual module, whether it comes from --virtual or the file.
	id, path = strings.TrimSpace(id), strings.TrimSpace(path)
	if id == "" || path == "" {
		return ErrVirtual
	}

	// A lone surrogate escape decodes to U+FFFD, so the file cannot say it either.
	if strings.ContainsRune(id+path, utf8.RuneError) {
		return ErrNotUTF8
	}

	if _, listed := virtual[id]; listed {
		return ErrDuplicateVirtual
	}

	virtual[id] = path

	return nil
}

func ParseTags(tags string) ([]string, error) {
	split := SplitTags(tags)

	if strings.ContainsRune(strings.Join(split, ""), utf8.RuneError) {
		return nil, ErrNotUTF8
	}

	return split, nil
}

func SplitTags(tags string) []string {
	// The one rule for build tags, whether they come from --tags or the file.
	var out []string

	for part := range strings.SplitSeq(tags, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}

	return out
}
