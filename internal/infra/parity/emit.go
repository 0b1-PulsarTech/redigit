package parity

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
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
		Position int     `json:"position"`
		Subject  string  `json:"subject"`
		Shape    string  `json:"shape"`
		Feature  string  `json:"feature"`
		Reason   string  `json:"reason"`
		IsWIP    bool    `json:"is_wip"`
		Chunks   []Chunk `json:"chunks"`
		Blobs    []Blob  `json:"blobs"`
		Risks    []Risk  `json:"risks"`
	}
)

func decodeRow(text []byte) (Row, error) {
	var row Row

	decoder := json.NewDecoder(bytes.NewReader(text))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&row); err != nil {
		return Row{}, fmt.Errorf("%w: emit row: %w", ErrMalformed, err)
	}

	// One row per line: anything after the first JSON value is corrupt input.
	if err := decoder.Decode(&json.RawMessage{}); !errors.Is(err, io.EOF) {
		return Row{}, fmt.Errorf("%w: emit row: trailing data after the row", ErrMalformed)
	}

	return row, nil
}

func StreamEmit(path string) iter.Seq2[Row, error] {
	return streamEmit(openFixture, path)
}

func streamEmit(open fixtureOpener, path string) iter.Seq2[Row, error] {
	return func(yield func(Row, error) bool) {
		isStopped := false

		err := forEachLineOpen(open, path, func(text []byte) error {
			row, decodeErr := decodeRow(text)
			if decodeErr != nil {
				return decodeErr
			}

			if !yield(row, nil) {
				isStopped = true

				return errStop
			}

			return nil
		})
		// After a false yield the iterator must not yield again (it would panic), so a close error is dropped on purpose.
		if err != nil && !isStopped {
			yield(Row{}, err)
		}
	}
}

func LoadEmit(path string) ([]Row, error) {
	var rows []Row

	for row, err := range StreamEmit(path) {
		if err != nil {
			return nil, err
		}

		rows = append(rows, row)
	}

	return rows, nil
}
