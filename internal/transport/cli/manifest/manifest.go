package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/rewriting"
)

type (
	Chunk struct {
		Key  string `json:"key"`
		Span [2]int `json:"span"`
		Unit string `json:"unit"`
	}

	Blob struct {
		Path   string `json:"path"`
		Mode   string `json:"mode"`
		Handle string `json:"handle"`
	}

	Risk struct {
		Chunk  string `json:"chunk"`
		Target string `json:"target"`
		BornAt int    `json:"born_at"`
	}

	Row struct {
		// One line of the manifest. Every list is present even when empty, so
		// a consumer never has to tell null from absent.
		Position int     `json:"position"`
		Subject  string  `json:"subject"`
		Shape    string  `json:"shape"`
		Feature  string  `json:"feature,omitempty"`
		Reason   string  `json:"reason,omitempty"`
		IsWIP    bool    `json:"is_wip"`
		Chunks   []Chunk `json:"chunks"`
		Blobs    []Blob  `json:"blobs"`
		Risks    []Risk  `json:"risks"`
	}
)

func Encode(out io.Writer, rows []Row) error {
	encoder := json.NewEncoder(out)

	for _, row := range rows {
		if row.Chunks == nil {
			row.Chunks = []Chunk{}
		}

		if row.Blobs == nil {
			row.Blobs = []Blob{}
		}

		if row.Risks == nil {
			row.Risks = []Risk{}
		}

		if err := encoder.Encode(row); err != nil {
			return fmt.Errorf("write manifest line: %w", err)
		}
	}

	return nil
}

func Decode(in io.Reader) ([]Row, error) {
	var rows []Row

	decoder := json.NewDecoder(in)

	for {
		var row Row

		if err := decoder.Decode(&row); errors.Is(err, io.EOF) {
			return rows, nil
		} else if err != nil {
			return nil, fmt.Errorf("%w: decode row: %w", rewriting.ErrManifest, err)
		}

		rows = append(rows, row)
	}
}
