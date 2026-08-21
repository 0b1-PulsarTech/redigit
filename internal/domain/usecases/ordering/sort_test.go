package ordering

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestCompareKeys(t *testing.T) {
	t.Parallel()

	a := entities.DeclKey{File: "a.go", Name: "A"}
	b := entities.DeclKey{File: "b.go", Name: "B"}

	testCases := []struct {
		name string
		a    entities.DeclKey
		b    entities.DeclKey
		want int
	}{
		{"earlier sorts before", a, b, -1},
		{"later sorts after", b, a, 1},
		{"equal keys compare equal", a, a, 0},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := compareKeys(testCase.a, testCase.b); got != testCase.want {
				t.Fatalf("compareKeys() = %d, want %d", got, testCase.want)
			}
		})
	}
}
