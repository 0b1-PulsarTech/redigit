package subjecting

import (
	"slices"
	"testing"
)

func TestID(t *testing.T) {
	t.Parallel()

	base := ID([]string{"a.go:func:A", "b.go:func:B"}, []string{"x.go", "y.go"})

	testCases := []struct {
		name      string
		chunkKeys []string
		blobPaths []string
		wantSame  bool
	}{
		{
			name:      "chunk reorder keeps the id",
			chunkKeys: []string{"b.go:func:B", "a.go:func:A"},
			blobPaths: []string{"x.go", "y.go"},
			wantSame:  true,
		},
		{
			name:      "blob reorder keeps the id",
			chunkKeys: []string{"a.go:func:A", "b.go:func:B"},
			blobPaths: []string{"y.go", "x.go"},
			wantSame:  true,
		},
		{
			name:      "a changed chunk changes the id",
			chunkKeys: []string{"a.go:func:A", "b.go:func:C"},
			blobPaths: []string{"x.go", "y.go"},
		},
		{
			name:      "a dropped chunk changes the id",
			chunkKeys: []string{"a.go:func:A"},
			blobPaths: []string{"x.go", "y.go"},
		},
		{
			name:      "a changed blob path changes the id",
			chunkKeys: []string{"a.go:func:A", "b.go:func:B"},
			blobPaths: []string{"x.go", "z.go"},
		},
		{
			name:      "a key moved across the chunk and blob boundary changes the id",
			chunkKeys: []string{"a.go:func:A"},
			blobPaths: []string{"b.go:func:B", "x.go", "y.go"},
		},
		{
			name:      "two keys joined into one change the id",
			chunkKeys: []string{"a.go:func:Ab.go:func:B"},
			blobPaths: []string{"x.go", "y.go"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := ID(testCase.chunkKeys, testCase.blobPaths)
			if len(got) != idLength {
				t.Fatalf("ID() = %q, want %d hex characters", got, idLength)
			}

			if (got == base) != testCase.wantSame {
				t.Fatalf("ID() = %q, base = %q, want same = %v", got, base, testCase.wantSame)
			}
		})
	}
}

func TestIDWithoutChunks(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		blobPaths []string
		other     []string
		wantSame  bool
	}{
		{
			name:      "blob paths alone give an id",
			blobPaths: []string{"go.mod"},
			other:     []string{"go.mod"},
			wantSame:  true,
		},
		{name: "other blob paths differ", blobPaths: []string{"go.mod"}, other: []string{"go.sum"}},
		{name: "no blob at all still hashes", blobPaths: nil, other: nil, wantSame: true},
		{name: "empty differs from a path", blobPaths: nil, other: []string{"go.mod"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			left, right := ID(nil, testCase.blobPaths), ID(nil, testCase.other)
			if len(left) != idLength || (left == right) != testCase.wantSame {
				t.Fatalf("ID() = %q and %q, want same = %v", left, right, testCase.wantSame)
			}
		})
	}
}

func TestDigest(t *testing.T) {
	t.Parallel()

	rows := []Row{
		{Position: 2, Subject: "feat(b): Done", Shape: "layer", IsWIP: true, Risks: 3},
		{
			Position: 1, Subject: "DRAFT: feat(a): x", Shape: "feature", Feature: "a",
			Reason: "why", ChunkKeys: []string{"k"}, BlobPaths: []string{"p"}, Units: []string{"u"},
		},
	}

	got := Digest(rows)

	positions := []int{got[0].Position, got[1].Position}
	if !slices.Equal(positions, []int{1, 2}) {
		t.Fatalf("Digest() positions = %v, want sorted", positions)
	}

	first, second := got[0], got[1]
	if first.ID != ID([]string{"k"}, []string{"p"}) || !first.IsDraft || first.Feature != "a" ||
		first.Reason != "why" || !slices.Equal(first.Units, []string{"u"}) {
		t.Fatalf("Digest()[0] = %+v", first)
	}

	if second.IsDraft || !second.IsWIP || second.Risks != 3 {
		t.Fatalf("Digest()[1] = %+v", second)
	}

	if second.Chunks == nil || second.Blobs == nil || second.Units == nil {
		t.Fatalf("Digest()[1] lists must be empty, never nil: %+v", second)
	}

	if len(Digest(nil)) != 0 {
		t.Fatal("Digest(nil) must be empty")
	}
}

func TestRowWritten(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		row     Row
		subject string
		want    string
	}{
		{
			name:    "a wip row marks",
			row:     Row{IsWIP: true},
			subject: ":bug: fix(a): x",
			want:    ":bug: fix(a): [wip] x",
		},
		{
			name:    "a plain row keeps what it is given",
			row:     Row{},
			subject: ":bug: fix(a): [wip] x",
			want:    ":bug: fix(a): [wip] x",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.row.Written(testCase.subject); got != testCase.want {
				t.Fatalf("Written() = %q, want %q", got, testCase.want)
			}
		})
	}
}
