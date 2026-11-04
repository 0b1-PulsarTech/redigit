package write

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/rewriting"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/subjecting"
)

type (
	blobLine struct {
		Path   string `json:"path"`
		Mode   string `json:"mode"`
		Handle string `json:"handle"`
	}

	rowLine struct {
		Position *int       `json:"position"`
		Subject  string     `json:"subject"`
		Blobs    []blobLine `json:"blobs"`
		IsWIP    bool       `json:"is_wip"`
		Chunks   []struct {
			Key string `json:"key"`
		} `json:"chunks"`
	}
)

func readRows(path string) ([]rewriting.Row, error) {
	file, err := os.Open(path) // #nosec G304 -- caller-supplied manifest path.
	if err != nil {
		return nil, fmt.Errorf("open manifest: %w", err)
	}

	defer func() { _ = file.Close() }()

	var rows []rewriting.Row

	decoder := json.NewDecoder(file)

	for {
		var line rowLine

		if err = decoder.Decode(&line); errors.Is(err, io.EOF) {
			return rows, nil
		} else if err != nil {
			return nil, fmt.Errorf("%w: decode row: %w", rewriting.ErrManifest, err)
		}

		if line.Position == nil {
			return nil, fmt.Errorf("%w: a row has no position", rewriting.ErrManifest)
		}

		row := rewriting.Row{Position: *line.Position, Subject: line.Subject, IsWIP: line.IsWIP}

		var paths []string

		for _, blob := range line.Blobs {
			if blob.Mode == "" {
				return nil, fmt.Errorf("%w: blob %q has no mode", rewriting.ErrManifest, blob.Path)
			}

			row.Blobs = append(row.Blobs, rewriting.BlobRef(blob))
			paths = append(paths, blob.Path)
		}

		row.ID = subjecting.ID(chunkKeys(line), paths)
		rows = append(rows, row)
	}
}

func readSubjects(path string) (map[string]string, error) {
	content, err := os.ReadFile(path) // #nosec G304 -- caller-supplied subjects path.
	if err != nil {
		return nil, fmt.Errorf("read subjects: %w", err)
	}

	var subjects map[string]string
	if err = json.Unmarshal(content, &subjects); err != nil {
		return nil, fmt.Errorf("%w: decode subjects: %w", rewriting.ErrManifest, err)
	}

	return subjects, nil
}

func chunkKeys(line rowLine) []string {
	keys := make([]string, 0, len(line.Chunks))
	for _, chunk := range line.Chunks {
		keys = append(keys, chunk.Key)
	}

	return keys
}
