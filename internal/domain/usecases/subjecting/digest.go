package subjecting

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

const idLength = 16

type (
	Row struct {
		Position  int
		Subject   string
		Shape     string
		Feature   string
		Reason    string
		IsWIP     bool
		ChunkKeys []string
		BlobPaths []string
		Units     []string
		Risks     int
	}

	Entry struct {
		Position int      `json:"position"`
		ID       string   `json:"id"`
		Subject  string   `json:"subject"`
		Shape    string   `json:"shape"`
		Feature  string   `json:"feature,omitempty"`
		Reason   string   `json:"reason,omitempty"`
		IsWIP    bool     `json:"is_wip"`
		IsDraft  bool     `json:"is_draft"`
		Risks    int      `json:"risks"`
		Units    []string `json:"units"`
		Chunks   []string `json:"chunks"`
		Blobs    []string `json:"blobs"`
	}
)

func ID(chunkKeys, blobPaths []string) string {
	hash := sha256.New()

	// Spans stay out: a line moves when a neighbour changes, the key does not.
	for _, key := range slices.Sorted(slices.Values(chunkKeys)) {
		_, _ = hash.Write([]byte(key))
		_, _ = hash.Write([]byte{0})
	}

	_, _ = hash.Write([]byte{1})

	for _, path := range slices.Sorted(slices.Values(blobPaths)) {
		_, _ = hash.Write([]byte(path))
		_, _ = hash.Write([]byte{0})
	}

	return hex.EncodeToString(hash.Sum(nil))[:idLength]
}

func Digest(rows []Row) []Entry {
	entries := make([]Entry, 0, len(rows))

	for _, row := range rows {
		entries = append(entries, Entry{
			Position: row.Position,
			ID:       ID(row.ChunkKeys, row.BlobPaths),
			Subject:  row.Subject,
			Shape:    row.Shape,
			Feature:  row.Feature,
			Reason:   row.Reason,
			IsWIP:    row.IsWIP,
			IsDraft:  entities.IsDraft(row.Subject),
			Risks:    row.Risks,
			Units:    nonNil(row.Units),
			Chunks:   nonNil(row.ChunkKeys),
			Blobs:    nonNil(row.BlobPaths),
		})
	}

	slices.SortStableFunc(entries, func(left, right Entry) int {
		return cmp.Compare(left.Position, right.Position)
	})

	return entries
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}

	return values
}

func (row Row) Written(subject string) string {
	if row.IsWIP {
		return Mark(subject)
	}

	return subject
}
