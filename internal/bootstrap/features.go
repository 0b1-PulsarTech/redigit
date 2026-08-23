package bootstrap

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// ErrMalformedFeature reports a line of a feature file the parser cannot read,
// and an overlap between two features. Both are refusals: a dropped or
// ambiguous slice silently changes which declarations a layer may reorder.
var ErrMalformedFeature = errors.New("malformed feature")

// LoadFeatures reads a feature file: one slice per line, the first and last
// position of the published history, then the name, "#" starting a comment.
//
//	0    12   bootstrap
//	243  245  agendamento   # engine and module born together
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
			"%w: %q covers %d-%d and %q covers %d-%d, so a shared position belongs to both",
			ErrMalformedFeature,
			first[0].Name, first[0].From, first[0].To,
			first[1].Name, first[1].From, first[1].To,
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

	const minFields = 3
	if len(fields) < minFields {
		return entities.Feature{}, false, fmt.Errorf(
			"%w: want a first position, a last position and a name, got %d field(s)",
			ErrMalformedFeature, len(fields),
		)
	}

	from, err := parsePosition(fields[0])
	if err != nil {
		return entities.Feature{}, false, err
	}

	to, err := parsePosition(fields[1])
	if err != nil {
		return entities.Feature{}, false, err
	}

	if to < from {
		return entities.Feature{}, false, fmt.Errorf(
			"%w: last position %d is before first position %d", ErrMalformedFeature, to, from,
		)
	}

	return entities.Feature{
		Name: strings.Join(fields[2:], " "),
		From: from,
		To:   to,
	}, true, nil
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
