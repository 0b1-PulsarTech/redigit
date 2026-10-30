package inputs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// ErrMalformedChunkMap reports a chunk map file the parser cannot read.
var ErrMalformedChunkMap = errors.New("malformed chunk map")

// chunkMapEntry is the wire shape of one entities.ChunkSpan.
type chunkMapEntry struct {
	Name string `json:"name"`
	Span [2]int `json:"span"`
}

// LoadChunkMap reads a chunk map file: a JSON object keyed by repo-relative
// path, each entry naming a piece an operator carved out of a format redigit
// cannot parse.
//
//	{"database/queries/lead.sql": [{"name": "GetLeadByID", "span": [0, 240]}]}
func LoadChunkMap(path string) (entities.ChunkMap, error) {
	content, err := os.ReadFile(path) // #nosec G304 -- caller-supplied config path.
	if err != nil {
		return nil, fmt.Errorf("read chunk map file: %w", err)
	}

	return ParseChunkMap(content)
}

// ParseChunkMap turns a chunk map file's bytes into entities.ChunkMap.
func ParseChunkMap(content []byte) (entities.ChunkMap, error) {
	var raw map[string][]chunkMapEntry

	if err := json.Unmarshal(content, &raw); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrMalformedChunkMap, err)
	}

	out := make(entities.ChunkMap, len(raw))

	for path, entries := range raw {
		spans := make([]entities.ChunkSpan, 0, len(entries))
		for _, entry := range entries {
			spans = append(spans, entities.ChunkSpan{
				Name: entry.Name,
				Span: entities.Span{Start: entry.Span[0], End: entry.Span[1]},
			})
		}

		out[path] = spans
	}

	return out, nil
}
