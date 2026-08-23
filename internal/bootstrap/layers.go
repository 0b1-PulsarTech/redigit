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

// ErrMalformedLayer reports a line of a layer file the parser cannot read.
// It is a refusal, not a skip: a typo that silently drops a rule changes the
// order of the whole rebuild without telling anyone.
var ErrMalformedLayer = errors.New("malformed layer rule")

// LoadLayers reads a layer file: one rule per line, the layer number then the
// path prefix, "#" starting a comment. Lower layers are born first.
//
//	1  libs/tereckernel
//	6  libs/infra   # persists module entities, so it sits after them
func LoadLayers(path string) (entities.Layers, error) {
	content, err := os.ReadFile(path) // #nosec G304 -- caller-supplied config path.
	if err != nil {
		return entities.Layers{}, fmt.Errorf("read layer file: %w", err)
	}

	return ParseLayers(content)
}

// ParseLayers turns a layer file's bytes into rules.
func ParseLayers(content []byte) (entities.Layers, error) {
	var rules []entities.LayerRule

	scanner := bufio.NewScanner(bytes.NewReader(content))

	for line := 1; scanner.Scan(); line++ {
		rule, ok, err := parseLayerLine(scanner.Text())
		if err != nil {
			return entities.Layers{}, fmt.Errorf("layer file line %d: %w", line, err)
		}

		if ok {
			rules = append(rules, rule)
		}
	}

	if err := scanner.Err(); err != nil {
		return entities.Layers{}, fmt.Errorf("scan layer file: %w", err)
	}

	return entities.NewLayers(rules), nil
}

func parseLayerLine(line string) (entities.LayerRule, bool, error) {
	if comment := strings.IndexByte(line, '#'); comment >= 0 {
		line = line[:comment]
	}

	fields := strings.Fields(line)
	if len(fields) == 0 {
		return entities.LayerRule{}, false, nil
	}

	if len(fields) != 2 {
		return entities.LayerRule{}, false, fmt.Errorf(
			"%w: want a layer number and a path prefix, got %d field(s)",
			ErrMalformedLayer, len(fields),
		)
	}

	layer, err := strconv.Atoi(fields[0])
	if err != nil {
		return entities.LayerRule{}, false, fmt.Errorf(
			"%w: %q is not a layer number", ErrMalformedLayer, fields[0],
		)
	}

	if layer < 0 {
		return entities.LayerRule{}, false, fmt.Errorf(
			"%w: layer %d is negative, and -1 already means unmatched",
			ErrMalformedLayer, layer,
		)
	}

	prefix := strings.Trim(fields[1], "/")
	if prefix == "" {
		return entities.LayerRule{}, false, fmt.Errorf(
			"%w: %q is not a path prefix", ErrMalformedLayer, fields[1],
		)
	}

	return entities.LayerRule{Prefix: prefix, Layer: entities.Layer(layer)}, true, nil
}
