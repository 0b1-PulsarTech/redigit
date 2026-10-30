package inputs

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// ErrMalformedFeature reports a line of a feature file the parser cannot read,
// and an overlap between two features. Both are refusals: a dropped or
// ambiguous slice silently changes which declarations a layer may reorder.
var ErrMalformedFeature = errors.New("malformed feature")

// LoadFeatures reads a feature file: one slice per line, the set of positions
// it covers, then the name, "#" starting a comment.
//
//	12-16,89-94  fundacao de erros
//	17,258,575   tags de lead   # schema, domain and use case, far apart
func LoadFeatures(path string) (entities.Features, error) {
	content, err := os.ReadFile(path) // #nosec G304 -- caller-supplied config path.
	if err != nil {
		return entities.Features{}, fmt.Errorf("read feature file: %w", err)
	}

	return ParseFeatures(content)
}

// ParseFeatures turns a feature file's bytes into slices, refusing any overlap.
func ParseFeatures(content []byte) (entities.Features, error) {
	var parsed []entities.Feature

	scanner := bufio.NewScanner(bytes.NewReader(content))

	for line := 1; scanner.Scan(); line++ {
		feature, ok, err := parseFeatureLine(scanner.Text())
		if err != nil {
			return entities.Features{}, fmt.Errorf("feature file line %d: %w", line, err)
		}

		if ok {
			parsed = append(parsed, feature)
		}
	}

	if err := scanner.Err(); err != nil {
		return entities.Features{}, fmt.Errorf("scan feature file: %w", err)
	}

	features := entities.NewFeatures(parsed)

	if overlaps := features.Overlaps(); len(overlaps) > 0 {
		first := overlaps[0]

		return entities.Features{}, fmt.Errorf(
			"%w: %q and %q both claim a position, so it belongs to both",
			ErrMalformedFeature, first[0].Name, first[1].Name,
		)
	}

	return features, nil
}

func parseFeatureLine(line string) (entities.Feature, bool, error) {
	if comment := strings.IndexByte(line, '#'); comment >= 0 {
		line = line[:comment]
	}

	fields := strings.Fields(line)
	if len(fields) == 0 {
		return entities.Feature{}, false, nil
	}

	const minFields = 2
	if len(fields) < minFields {
		return entities.Feature{}, false, fmt.Errorf(
			"%w: want a set of positions and a name, got %d field(s)",
			ErrMalformedFeature, len(fields),
		)
	}

	at, err := parsePositionSet(fields[0])
	if err != nil {
		return entities.Feature{}, false, err
	}

	return entities.Feature{Name: strings.Join(fields[1:], " "), At: at}, true, nil
}

// parsePositionSet reads "17,258,575" and "12-16,89-94" and mixtures of the
// two into the positions they name, sorted and deduplicated.
//
// A capability is a set because the published history built it in pieces that
// sit far apart: a range spanning them would swallow everything in between.
func parsePositionSet(spec string) ([]entities.Position, error) {
	seen := map[entities.Position]bool{}

	for part := range strings.SplitSeq(spec, ",") {
		if part = strings.TrimSpace(part); part == "" {
			return nil, fmt.Errorf("%w: %q holds an empty position", ErrMalformedFeature, spec)
		}

		from, to, err := parseRange(part)
		if err != nil {
			return nil, err
		}

		for at := from; at <= to; at++ {
			seen[at] = true
		}
	}

	at := slices.Collect(maps.Keys(seen))
	slices.Sort(at)

	return at, nil
}

// parseRange reads one element of a set: "42" or "12-16". Both ends are
// inclusive.
func parseRange(part string) (from, to entities.Position, err error) {
	first, last, isRange := strings.Cut(part, "-")
	if !isRange {
		at, atErr := parsePosition(part)

		return at, at, atErr
	}

	if from, err = parsePosition(first); err != nil {
		return 0, 0, err
	}

	if to, err = parsePosition(last); err != nil {
		return 0, 0, err
	}

	if to < from {
		return 0, 0, fmt.Errorf(
			"%w: range %q ends before it starts", ErrMalformedFeature, part,
		)
	}

	return from, to, nil
}

func parsePosition(field string) (entities.Position, error) {
	at, err := strconv.Atoi(field)
	if err != nil {
		return 0, fmt.Errorf("%w: %q is not a position", ErrMalformedFeature, field)
	}

	if at < 0 {
		return 0, fmt.Errorf("%w: position %d is negative", ErrMalformedFeature, at)
	}

	return entities.Position(at), nil
}
