package subjects

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/rewriting"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/subjecting"
	"github.com/wrapped-owls/redigit/internal/transport/cli/manifest"
)

func loadRows(path string) ([]subjecting.Row, error) {
	file, err := os.Open(path) // #nosec G304 -- caller-supplied manifest path.
	if err != nil {
		return nil, fmt.Errorf("open manifest: %w", err)
	}

	defer func() { _ = file.Close() }()

	decoded, err := manifest.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}

	rows := make([]subjecting.Row, 0, len(decoded))
	for _, row := range decoded {
		rows = append(rows, toRow(row))
	}

	return rows, nil
}

func toRow(row manifest.Row) subjecting.Row {
	out := subjecting.Row{
		Position: row.Position, Subject: row.Subject, Shape: row.Shape, Feature: row.Feature,
		Reason: row.Reason, IsWIP: row.IsWIP, Risks: len(row.Risks),
	}

	for _, chunk := range row.Chunks {
		out.ChunkKeys = append(out.ChunkKeys, chunk.Key)
		out.Units = append(out.Units, chunk.Unit)
	}

	for _, blob := range row.Blobs {
		out.BlobPaths = append(out.BlobPaths, blob.Path)
	}

	out.Units = slices.Compact(slices.Sorted(slices.Values(out.Units)))

	return out
}

func loadSubjects(path string) (map[string]string, error) {
	content, err := os.ReadFile(path) // #nosec G304 -- caller-supplied subjects path.
	if err != nil {
		return nil, fmt.Errorf("read subjects: %w", err)
	}

	var loaded map[string]string
	if err = json.Unmarshal(content, &loaded); err != nil {
		return nil, fmt.Errorf("%w: decode subjects: %w", rewriting.ErrSubjects, err)
	}

	return loaded, nil
}

func loadDigest(path string) ([]subjecting.Entry, error) {
	file, err := os.Open(path) // #nosec G304 -- caller-supplied digest path.
	if err != nil {
		return nil, fmt.Errorf("open digest: %w", err)
	}

	defer func() { _ = file.Close() }()

	var entries []subjecting.Entry

	decoder := json.NewDecoder(file)

	for {
		var entry subjecting.Entry

		if err = decoder.Decode(&entry); errors.Is(err, io.EOF) {
			return entries, nil
		} else if err != nil {
			return nil, fmt.Errorf("%w: decode digest: %w", rewriting.ErrManifest, err)
		}

		entries = append(entries, entry)
	}
}

func writeJSON(path string, value any) error {
	const fileMode fs.FileMode = 0o600

	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}

	if err = os.WriteFile(path, append(content, '\n'), fileMode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	return nil
}

func orEmpty[Item any](values []Item) []Item {
	if values == nil {
		return []Item{}
	}

	return values
}
