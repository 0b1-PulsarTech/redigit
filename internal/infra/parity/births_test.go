package parity

import (
	"errors"
	"slices"
	"testing"
)

func TestBirths(t *testing.T) {
	t.Parallel()

	rows := []Row{
		{Position: 2, Chunks: []Chunk{{Key: "b"}, {Key: "a"}, {Key: "a"}}},
		{Position: 1, Chunks: []Chunk{{Key: "a"}}},
		{Position: 3, Chunks: []Chunk{{Key: "c"}}},
	}

	first := firstPositions(rows)
	if first["a"] != 1 || first["b"] != 2 || first["c"] != 3 {
		t.Fatalf("first = %v", first)
	}

	sets, err := keySets(rows)
	if err != nil {
		t.Fatalf("keySets: %v", err)
	}

	if !slices.Equal(sets[2], []string{"a", "b"}) || !slices.Equal(sets[1], []string{"a"}) {
		t.Fatalf("sets = %v", sets)
	}
}

func TestKeySetsRejectsDuplicatePosition(t *testing.T) {
	t.Parallel()

	_, err := keySets([]Row{rowOf(2, "a"), rowOf(2, "b")})
	if !errors.Is(err, ErrMalformed) {
		t.Fatalf("err = %v, want ErrMalformed", err)
	}
}
