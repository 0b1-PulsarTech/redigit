package pinrule

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type (
	segment struct {
		text string
		meta bool
	}

	pattern struct {
		text string
		file segment
		kind segment
		name segment
	}
)

func parsePattern(text string) (pattern, error) {
	const patternParts = 3

	parts := strings.SplitN(text, ":", patternParts)
	if len(parts) != patternParts || slices.Contains(parts, "") {
		return pattern{}, fmt.Errorf("%q: %w: want file:kind:name", text, ErrBadPattern)
	}

	segments := make([]segment, len(parts))

	for idx, part := range parts {
		if _, err := path.Match(part, ""); err != nil {
			return pattern{}, fmt.Errorf("%q: %w: %w", text, ErrBadPattern, err)
		}

		segments[idx] = segment{text: part, meta: strings.ContainsAny(part, `*?[\`)}
	}

	return pattern{text: text, file: segments[0], kind: segments[1], name: segments[2]}, nil
}

func (pat pattern) matches(key entities.DeclKey) bool {
	return pat.file.matches(key.File) && pat.kind.matches(string(key.Kind)) &&
		pat.name.matches(key.Name)
}

func (seg segment) matches(value string) bool {
	if !seg.meta {
		return seg.text == value
	}

	ok, err := path.Match(seg.text, value)

	return err == nil && ok
}

func parseTarget(text string) (pattern, error) {
	pat, err := parsePattern(text)
	if err != nil {
		return pattern{}, err
	}

	if pat.kind.text == string(entities.KindStmt) {
		return pattern{}, fmt.Errorf(
			"%q: %w: a pin names a whole declaration and cut statements do not exist "+
				"yet when rules first run, so name the func", text, ErrStmtPattern,
		)
	}

	return pat, nil
}
