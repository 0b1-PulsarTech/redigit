package lineage

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func hashes(tokens ...string) []uint64 {
	out := make([]uint64, 0, len(tokens))
	for _, token := range tokens {
		out = append(out, hashToken(token))
	}

	return out
}

func TestTokenize(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		text string
		want []uint64
	}{
		{"empty text", "", []uint64{}},
		{"blank text", " \t\r\n ", []uint64{}},
		{"words and punctuation", "func Foo(a_1 int) {", hashes(
			"func", "Foo", "(", "a_1", "int", ")", "{")},
		{"whitespace does not matter", "a\n\t b", hashes("a", "b")},
		{"non ascii stays inside a word", "caf\u00e9 x", hashes("caf\u00e9", "x")},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := tokenize([]byte(testCase.text)); !slices.Equal(got, testCase.want) {
				t.Fatalf("tokenize(%q) = %v, want %v", testCase.text, got, testCase.want)
			}
		})
	}
}

func words(count, from int) []byte {
	var out strings.Builder
	for idx := from; idx < from+count; idx++ {
		fmt.Fprintf(&out, "w%d ", idx)
	}

	return []byte(out.String())
}

func TestJaccard(t *testing.T) {
	t.Parallel()

	mixed := append(words(4, 0), words(2, 50)...)

	testCases := []struct {
		name        string
		left, right []byte
		want        int
		wantSimilar bool
	}{
		{"both empty", nil, nil, 0, false},
		{"one empty", words(10, 0), nil, 0, false},
		{"identical", words(10, 0), words(10, 0), 100, true},
		{"identical short text", []byte("a b"), []byte("a b"), 100, true},
		{"disjoint", words(10, 0), words(10, 100), 0, false},
		{"partial overlap below the floor", words(5, 0), mixed, 40, false},
		{"just above the floor", words(13, 0), words(13, 0)[:len(words(12, 0))], 90, true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			left := shinglesOf(tokenize(testCase.left))
			right := shinglesOf(tokenize(testCase.right))

			got, isSimilar := similarity(left, right)
			if got != testCase.want {
				t.Fatalf("similarity = %d, want %d", got, testCase.want)
			}

			if isSimilar != testCase.wantSimilar {
				t.Fatalf("isSimilar = %v, want %v", isSimilar, testCase.wantSimilar)
			}
		})
	}
}

func TestSimilarBoundary(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		shared      int
		onlyLeft    int
		wantSimilar bool
	}{
		{"60 percent is similar", 6, 4, true},
		{"59 percent is not", 59, 41, false},
		{"61 percent is similar", 61, 39, true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var left, right shingles

			for idx := range testCase.shared {
				left = append(left, uint64(idx))
				right = append(right, uint64(idx))
			}

			for idx := range testCase.onlyLeft {
				left = append(left, uint64(1000+idx))
			}

			if _, isSimilar := similarity(left, right); isSimilar != testCase.wantSimilar {
				t.Fatalf("isSimilar = %v, want %v", isSimilar, testCase.wantSimilar)
			}
		})
	}
}

func BenchmarkJaccard(b *testing.B) {
	left, right := words(500, 0), words(500, 100)
	leftTokens, rightTokens := tokenize(left), tokenize(right)

	b.ReportAllocs()

	var sink int

	for b.Loop() {
		score, _ := similarity(shinglesOf(leftTokens), shinglesOf(rightTokens))
		sink += score
	}

	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/1000, "us/pair")

	if sink < 0 {
		b.Fatal("unreachable")
	}
}

func hashToken(token string) uint64 {
	hash := uint64(fnvOffset)
	for idx := range len(token) {
		hash ^= uint64(token[idx])
		hash *= fnvPrime
	}

	return hash
}
