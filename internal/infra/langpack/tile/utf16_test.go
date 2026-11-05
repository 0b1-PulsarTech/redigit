package tile

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"unicode/utf16"
)

const (
	mixedSource = "a\xc3\xa9\xe2\x82\xac\U0001F600z"
	bmpEdgePair = "\xef\xbf\xbf\U00010000"
	bomA        = "\xef\xbb\xbfa"
)

type offsetCase struct {
	name      string
	source    string
	offsets   []int
	want      []int
	wantErr   error
	wantIndex int
}

func TestUTF16ToBytes(t *testing.T) {
	t.Parallel()

	testCases := []offsetCase{
		{"empty offsets", mixedSource, nil, []int{}, nil, 0},
		{"ascii", "hello", []int{0, 2, 5}, []int{0, 2, 5}, nil, 0},
		{"two byte rune", mixedSource, []int{1, 2}, []int{1, 3}, nil, 0},
		{"three byte rune", mixedSource, []int{2, 3}, []int{3, 6}, nil, 0},
		{"surrogate pair", mixedSource, []int{3, 5}, []int{6, 10}, nil, 0},
		{"end of source", mixedSource, []int{6}, []int{11}, nil, 0},
		{"unsorted keeps input order", mixedSource, []int{6, 0, 3, 3}, []int{11, 0, 6, 6}, nil, 0},
		{"bmp edge then pair", bmpEdgePair, []int{1, 3}, []int{3, 7}, nil, 0},
		{"inside a pair after bmp edge", bmpEdgePair, []int{2}, nil, ErrOffsetInsideRune, 0},
		{"bom is one unit and three bytes", bomA, []int{0, 1, 2}, []int{0, 3, 4}, nil, 0},
		{"inside a pair", mixedSource, []int{0, 4}, nil, ErrOffsetInsideRune, 1},
		{"inside a pair after sorting", mixedSource, []int{4, 0}, nil, ErrOffsetInsideRune, 0},
		{"past the end", mixedSource, []int{7}, nil, ErrOffsetRange, 0},
		{"past the end of empty", "", []int{1}, nil, ErrOffsetRange, 0},
		{"negative", "abc", []int{1, -1}, nil, ErrOffsetRange, 1},
		{
			"lowest input index wins over sorted order",
			mixedSource,
			[]int{7, 4},
			nil,
			ErrOffsetRange,
			0,
		},
		{
			"lowest input index when the larger is first",
			mixedSource,
			[]int{99, 4},
			nil,
			ErrOffsetRange,
			0,
		},
		{
			"lowest input index with a valid sibling",
			mixedSource,
			[]int{99, 2},
			nil,
			ErrOffsetRange,
			0,
		},
		{
			"lowest input index inside a pair",
			mixedSource,
			[]int{4, 99},
			nil,
			ErrOffsetInsideRune,
			0,
		},
		{"duplicate bad offsets", mixedSource, []int{4, 4}, nil, ErrOffsetInsideRune, 0},
		{"duplicate past the end", mixedSource, []int{99, 99}, nil, ErrOffsetRange, 0},
		{"invalid byte is one unit", "a\xffb", []int{2, 3}, []int{2, 3}, nil, 0},
		{"truncated rune is one unit per byte", "\xe2\x82", []int{1, 2}, []int{1, 2}, nil, 0},
		{"real U+FFFD is one unit", "\xef\xbf\xbdb", []int{1}, []int{3}, nil, 0},
		{"invalid byte past the end", "a\xff", []int{3}, nil, ErrOffsetRange, 0},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := UTF16ToBytes([]byte(testCase.source), testCase.offsets)
			assertOffsets(t, got, err, testCase)
		})
	}
}

func TestBytesToUTF16(t *testing.T) {
	t.Parallel()

	testCases := []offsetCase{
		{"empty offsets", mixedSource, nil, []int{}, nil, 0},
		{"ascii", "hello", []int{0, 2, 5}, []int{0, 2, 5}, nil, 0},
		{"two byte rune", mixedSource, []int{1, 3}, []int{1, 2}, nil, 0},
		{"three byte rune", mixedSource, []int{6}, []int{3}, nil, 0},
		{"four byte rune", mixedSource, []int{6, 10}, []int{3, 5}, nil, 0},
		{"end of source", mixedSource, []int{11}, []int{6}, nil, 0},
		{"unsorted keeps input order", mixedSource, []int{11, 0, 6, 6}, []int{6, 0, 3, 3}, nil, 0},
		{"bmp edge then pair", bmpEdgePair, []int{3, 7}, []int{1, 3}, nil, 0},
		{"bom is one unit and three bytes", bomA, []int{0, 3, 4}, []int{0, 1, 2}, nil, 0},
		{"inside a bom", bomA, []int{1}, nil, ErrOffsetInsideRune, 0},
		{"inside a two byte rune", mixedSource, []int{2}, nil, ErrOffsetInsideRune, 0},
		{"inside a four byte rune", mixedSource, []int{0, 8}, nil, ErrOffsetInsideRune, 1},
		{"past the end", mixedSource, []int{12}, nil, ErrOffsetRange, 0},
		{"negative", "abc", []int{-1}, nil, ErrOffsetRange, 0},
		{
			"lowest input index wins over sorted order",
			mixedSource,
			[]int{12, 8},
			nil,
			ErrOffsetRange,
			0,
		},
		{
			"lowest input index when the larger is first",
			mixedSource,
			[]int{99, 2},
			nil,
			ErrOffsetRange,
			0,
		},
		{
			"lowest input index with a valid sibling",
			mixedSource,
			[]int{99, 3},
			nil,
			ErrOffsetRange,
			0,
		},
		{
			"lowest input index inside a rune",
			mixedSource,
			[]int{2, 99},
			nil,
			ErrOffsetInsideRune,
			0,
		},
		{"duplicate bad offsets", mixedSource, []int{2, 2}, nil, ErrOffsetInsideRune, 0},
		{"duplicate past the end", mixedSource, []int{99, 99}, nil, ErrOffsetRange, 0},
		{"invalid bytes are boundaries", "a\xff\xfeb", []int{1, 2, 3}, []int{1, 2, 3}, nil, 0},
		{"truncated rune is one unit per byte", "\xe2\x82", []int{1}, []int{1}, nil, 0},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := BytesToUTF16([]byte(testCase.source), testCase.offsets)
			assertOffsets(t, got, err, testCase)
		})
	}
}

func TestUTF16RoundTrip(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		source string
		bounds []int
	}{
		{"mixed with an invalid byte", mixedSource + "\xff" + mixedSource, []int{
			0, 1, 3, 6, 10, 11, 12, 13, 15, 18, 22, 23,
		}},
		{"ascii", "hello", []int{0, 1, 5}},
		{"bom", bomA + "bc", []int{0, 3, 6}},
		{"empty", "", []int{0}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			source := []byte(testCase.source)

			units, err := BytesToUTF16(source, testCase.bounds)
			if err != nil {
				t.Fatalf("bytes to utf16: %v", err)
			}

			got, err := UTF16ToBytes(source, units)
			if err != nil {
				t.Fatalf("utf16 to bytes: %v", err)
			}

			if !slices.Equal(got, testCase.bounds) {
				t.Fatalf("round trip = %v, want %v", got, testCase.bounds)
			}
		})
	}
}

func assertOffsets(tb testing.TB, got []int, err error, testCase offsetCase) {
	tb.Helper()

	if !errors.Is(err, testCase.wantErr) {
		tb.Fatalf("err = %v, want %v", err, testCase.wantErr)
	}

	if testCase.wantErr != nil {
		indexErr, isIndexErr := errors.AsType[*IndexError](err)
		if !isIndexErr || indexErr.Index != testCase.wantIndex {
			tb.Fatalf("err = %#v, want index %d", err, testCase.wantIndex)
		}

		return
	}

	if !slices.Equal(got, testCase.want) {
		tb.Fatalf("got %v, want %v", got, testCase.want)
	}
}

func FuzzUTF16(f *testing.F) {
	f.Add([]byte(mixedSource))
	f.Add([]byte(bomA + "\xff\xe2\x82b"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, source []byte) {
		var (
			byteBounds = make([]int, 0, len(source)+1)
			unitBounds = make([]int, 0, len(source)+1)
			units      int
		)

		for pos, char := range string(source) {
			byteBounds = append(byteBounds, pos)
			unitBounds = append(unitBounds, units)
			units += len(utf16.Encode([]rune{char}))
		}

		byteBounds = append(byteBounds, len(source))
		unitBounds = append(unitBounds, units)

		if want := len(utf16.Encode([]rune(string(source)))); units != want {
			t.Fatalf("oracle disagrees with itself: %d vs %d", units, want)
		}

		gotUnits, err := BytesToUTF16(source, byteBounds)
		if err != nil || !slices.Equal(gotUnits, unitBounds) {
			t.Fatalf("bytes to utf16 = %v, %v; want %v (%q)", gotUnits, err, unitBounds, source)
		}

		gotBytes, err := UTF16ToBytes(source, unitBounds)
		if err != nil || !slices.Equal(gotBytes, byteBounds) {
			t.Fatalf("utf16 to bytes = %v, %v; want %v (%q)", gotBytes, err, byteBounds, source)
		}
	})
}

func BenchmarkUTF16ToBytes(b *testing.B) {
	source := []byte(strings.Repeat(mixedSource+"\n", 1<<20/len(mixedSource+"\n")))
	offsets := make([]int, 0, len(source)/14)

	for off := 0; off < len(source)/2; off += 7 {
		offsets = append(offsets, off)
	}

	b.SetBytes(int64(len(source)))
	b.ReportAllocs()

	for range b.N {
		if _, err := UTF16ToBytes(source, offsets); err != nil {
			b.Fatalf("convert: %v", err)
		}
	}
}
