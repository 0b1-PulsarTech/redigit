package subjects

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/rewriting"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/subjecting"
)

func TestDigestCommand(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		manifest string
		wantRows int
		wantErr  error
	}{
		{name: "two commits", manifest: rowOne + rowZero, wantRows: 2},
		{name: "an empty manifest", manifest: "", wantRows: 0},
		{name: "garbage", manifest: "{nope", wantErr: rewriting.ErrManifest},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			path := writeTemp(t, "m.jsonl", testCase.manifest)

			var stdout bytes.Buffer

			err := Run(context.Background(), []string{"digest", "--manifest", path}, &stdout)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Run() err = %v, want %v", err, testCase.wantErr)
			}

			if testCase.wantErr != nil {
				return
			}

			var entries []subjecting.Entry

			decoder := json.NewDecoder(strings.NewReader(stdout.String()))
			for decoder.More() {
				var entry subjecting.Entry
				if err = decoder.Decode(&entry); err != nil {
					t.Fatalf("decode digest line: %v", err)
				}

				entries = append(entries, entry)
			}

			if len(entries) != testCase.wantRows {
				t.Fatalf("digest rows = %d, want %d", len(entries), testCase.wantRows)
			}

			if testCase.wantRows == 2 && (entries[0].Position != 0 || !entries[1].IsWIP ||
				entries[1].Risks != 1 || entries[0].ID != subjecting.ID([]string{"k1", "k2"}, []string{"a.go"}) ||
				len(entries[0].Units) != 1) {
				t.Fatalf("digest entries = %+v", entries)
			}
		})
	}
}

func TestDigestCommandMissingManifest(t *testing.T) {
	t.Parallel()

	args := []string{"digest", "--manifest", filepath.Join(t.TempDir(), "absent")}

	var stdout bytes.Buffer

	if err := Run(context.Background(), args, &stdout); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Run() err = %v, want %v", err, fs.ErrNotExist)
	}
}

func TestDigestCommandWriteFailure(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		manifest string
	}{
		{name: "a digest line cannot be written", manifest: rowZero},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			args := []string{"digest", "--manifest", writeTemp(t, "m.jsonl", testCase.manifest)}

			if err := Run(context.Background(), args, failingWriter{}); !errors.Is(
				err,
				errWriterBroken,
			) {
				t.Fatalf("Run() err = %v, want %v", err, errWriterBroken)
			}
		})
	}
}
