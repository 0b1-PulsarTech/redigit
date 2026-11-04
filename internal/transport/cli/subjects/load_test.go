package subjects

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/rewriting"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/subjecting"
	"github.com/wrapped-owls/redigit/internal/transport/cli/manifest"
)

func TestToRow(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		row  manifest.Row
		want subjecting.Row
	}{
		{
			name: "keys keep their order, units are sorted and compacted",
			row: manifest.Row{
				Position: 3,
				Subject:  "s",
				IsWIP:    true,
				Chunks: []manifest.Chunk{
					{Key: "k2", Unit: "b"},
					{Key: "k1", Unit: "a"},
					{Key: "k3", Unit: "b"},
				},
				Blobs: []manifest.Blob{{Path: "p"}},
				Risks: []manifest.Risk{{}, {}},
			},
			want: subjecting.Row{
				Position: 3, Subject: "s", IsWIP: true, Risks: 2,
				ChunkKeys: []string{"k2", "k1", "k3"}, Units: []string{"a", "b"},
				BlobPaths: []string{"p"},
			},
		},
		{
			name: "a row with nothing in it stays empty",
			row:  manifest.Row{Position: 1},
			want: subjecting.Row{Position: 1},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := toRow(testCase.row)
			if got.Position != testCase.want.Position || got.Subject != testCase.want.Subject ||
				got.IsWIP != testCase.want.IsWIP || got.Risks != testCase.want.Risks ||
				!slices.Equal(got.ChunkKeys, testCase.want.ChunkKeys) ||
				!slices.Equal(got.Units, testCase.want.Units) ||
				!slices.Equal(got.BlobPaths, testCase.want.BlobPaths) {
				t.Fatalf("toRow() = %+v, want %+v", got, testCase.want)
			}
		})
	}
}

func TestLoadDigestAndSubjectsFailures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		run     func(t *testing.T) error
		wantErr error
		isAny   bool
	}{
		{
			name: "a garbage digest",
			run: func(t *testing.T) error {
				t.Helper()

				_, err := loadDigest(writeTemp(t, "d", "{nope"))

				return err
			},
			wantErr: rewriting.ErrManifest,
		},
		{
			name: "subjects that are not an object",
			run: func(t *testing.T) error {
				t.Helper()

				_, err := loadSubjects(writeTemp(t, "s", "[1]"))

				return err
			},
			wantErr: rewriting.ErrSubjects,
		},
		{
			name: "a json file in a missing directory",
			run: func(t *testing.T) error {
				t.Helper()

				return writeJSON(filepath.Join(t.TempDir(), "x", "y"), 1)
			},
			isAny: true,
		},
		{
			name: "a value json cannot encode",
			run: func(t *testing.T) error {
				t.Helper()

				return writeJSON(filepath.Join(t.TempDir(), "x"), make(chan int))
			},
			isAny: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := testCase.run(t)
			if testCase.isAny && err == nil {
				t.Fatal("want an error, got none")
			}

			if !testCase.isAny && !errors.Is(err, testCase.wantErr) {
				t.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestOrEmpty(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input []int
		want  []int
	}{
		{name: "nil becomes an empty slice", input: nil, want: []int{}},
		{name: "a slice is returned as it is", input: []int{1}, want: []int{1}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := orEmpty(testCase.input)
			if got == nil || !slices.Equal(got, testCase.want) {
				t.Fatalf("orEmpty(%v) = %#v, want %#v", testCase.input, got, testCase.want)
			}
		})
	}
}
