package batch

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

func parseEntry(reader *bufio.Reader, meta string) (Entry, error) {
	fields := strings.Fields(strings.TrimPrefix(meta, ":"))
	if !strings.HasPrefix(meta, ":") || len(fields) != 5 {
		return Entry{}, fmt.Errorf("%w: raw record %q", ErrMalformedLog, meta)
	}

	entry := Entry{
		OldMode: fields[0], NewMode: fields[1],
		OldBlob: fields[2], NewBlob: fields[3],
		Status: fields[4][0],
	}

	if len(fields[4]) > 1 {
		score, err := strconv.Atoi(fields[4][1:])
		if err != nil {
			return Entry{}, fmt.Errorf("%w: similarity in %q", ErrMalformedLog, meta)
		}

		entry.Similarity = score
	}

	first, err := readPath(reader)
	if err != nil {
		return Entry{}, err
	}

	if entry.Status != 'R' && entry.Status != 'C' {
		entry.OldPath, entry.NewPath = first, first

		return entry, nil
	}

	second, err := readPath(reader)
	if err != nil {
		return Entry{}, err
	}

	entry.OldPath, entry.NewPath = first, second

	return entry, nil
}

func readPath(reader *bufio.Reader) (string, error) {
	path, err := reader.ReadBytes(0)
	if err != nil {
		return "", fmt.Errorf("%w: path missing: %w", ErrMalformedLog, err)
	}

	return string(bytes.TrimSuffix(path, []byte{0})), nil
}
