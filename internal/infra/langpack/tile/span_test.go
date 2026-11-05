package tile

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

type testItem struct {
	text      string
	isContent bool
}

func spansOf(tb testing.TB, source string, items ...testItem) []Span {
	tb.Helper()

	spans := make([]Span, 0, len(items))
	from := 0

	for _, item := range items {
		at := strings.Index(source[from:], item.text)
		if at < 0 {
			tb.Fatalf("item %q not found in source", item.text)
		}

		start := from + at
		from = start + len(item.text)
		spans = append(spans, Span{Start: start, End: from, IsContent: item.isContent})
	}

	return spans
}

func keepOnly(indexes ...int) func(int) bool {
	return func(index int) bool { return slices.Contains(indexes, index) }
}

type tileCase struct {
	name   string
	source string
	items  []testItem
	spans  []Span
	keep   func(int) bool
	want   []byte
}

func TestTile(t *testing.T) {
	t.Parallel()

	const (
		lfSource   = "// head\n\nimport a\n\n\nfunc b\n\nfunc c\n// tail\n"
		crlfSource = "import a\r\n\r\nfunc b\r\n"
	)

	lfItems := []testItem{{"import a", false}, {"func b", true}, {"func c", true}}
	crlfItems := []testItem{{"import a", false}, {"func b", true}}

	testCases := []tileCase{
		{"keep all", lfSource, lfItems, nil, keepOnly(0, 1, 2), []byte(lfSource)},
		{
			"drop first", lfSource, lfItems, nil, keepOnly(1, 2),
			[]byte("// head\n\n\n\n\nfunc b\n\nfunc c\n// tail\n"),
		},
		{
			"drop middle takes its blank lines", lfSource, lfItems, nil, keepOnly(0, 2),
			[]byte("// head\n\nimport a\n\nfunc c\n// tail\n"),
		},
		{
			"drop last", lfSource, lfItems, nil, keepOnly(0, 1),
			[]byte("// head\n\nimport a\n\n\nfunc b\n// tail\n"),
		},
		{"only non-content kept", lfSource, lfItems, nil, keepOnly(0), nil},
		{"nothing kept", lfSource, lfItems, nil, keepOnly(), nil},
		{"keep all crlf", crlfSource, crlfItems, nil, keepOnly(0, 1), []byte(crlfSource)},
		{
			"first gap loses leading crlf", crlfSource, crlfItems, nil, keepOnly(1),
			[]byte("func b\r\n"),
		},
		{"empty spans", "anything", nil, nil, keepOnly(), nil},
		{
			"zero-width first span keeps identity", "\n\nfoo", nil,
			[]Span{{Start: 0, End: 0}, {Start: 2, End: 5, IsContent: true}},
			keepOnly(0, 1), []byte("\n\nfoo"),
		},
	}

	runTileCases(t, testCases)
}

func TestTile_Edges(t *testing.T) {
	t.Parallel()

	testCases := []tileCase{
		{
			"keep all content then non-content", "func b\n\nimport a\n",
			[]testItem{{"func b", true}, {"import a", false}},
			nil,
			keepOnly(0, 1), []byte("func b\n\nimport a\n"),
		},
		{
			"gap indentation survives a dropped first item", "import a\n\n  func b",
			[]testItem{{"import a", false}, {"func b", true}},
			nil,
			keepOnly(1), []byte("  func b"),
		},
		{
			"subset keeping a zero-width span", "\n\nimport a\n\nfunc b", nil,
			[]Span{{Start: 0, End: 0}, {Start: 2, End: 10}, {Start: 12, End: 18, IsContent: true}},
			keepOnly(0, 2), []byte("func b"),
		},
		{
			"subset without the zero-width span", "\n\nimport a\n\nfunc b", nil,
			[]Span{{Start: 0, End: 0}, {Start: 2, End: 10}, {Start: 12, End: 18, IsContent: true}},
			keepOnly(2), []byte("func b"),
		},
		{
			"whole source kept", "abc", nil,
			[]Span{{Start: 0, End: 3, IsContent: true}},
			keepOnly(0), []byte("abc"),
		},
		{
			"whole source dropped", "abc", nil,
			[]Span{{Start: 0, End: 3, IsContent: true}},
			keepOnly(), nil,
		},
		{
			"adjacent spans drop first", "abcd", nil,
			[]Span{{Start: 0, End: 2, IsContent: true}, {Start: 2, End: 4, IsContent: true}},
			keepOnly(1), []byte("cd"),
		},
		{
			"adjacent spans drop last", "abcd", nil,
			[]Span{{Start: 0, End: 2, IsContent: true}, {Start: 2, End: 4, IsContent: true}},
			keepOnly(0), []byte("ab"),
		},
	}

	runTileCases(t, testCases)
}

func runTileCases(t *testing.T, testCases []tileCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			spans := testCase.spans
			if spans == nil {
				spans = spansOf(t, testCase.source, testCase.items...)
			}

			got := Tile([]byte(testCase.source), spans, testCase.keep)
			if !bytes.Equal(got, testCase.want) || (got == nil) != (testCase.want == nil) {
				t.Fatalf("got %q, want %q", got, testCase.want)
			}
		})
	}
}

func referenceTile(source []byte, spans []Span, keep func(int) bool) []byte {
	if len(spans) == 0 {
		return nil
	}

	var (
		out        = slices.Clone(source[:spans[0].Start])
		hasDropped bool
		hasContent bool
	)

	for index, span := range spans {
		gapStart := span.Start
		if index > 0 {
			gapStart = spans[index-1].End
		}

		if !keep(index) {
			hasDropped = true

			continue
		}

		gap := string(source[gapStart:span.Start])
		if len(out) == 0 && hasDropped {
			gap = strings.TrimLeft(gap, "\r\n")
		}

		out = append(out, gap...)
		out = append(out, source[span.Start:span.End]...)
		hasContent = hasContent || span.IsContent
	}

	if !hasContent {
		return nil
	}

	return append(out, source[spans[len(spans)-1].End:]...)
}

func fuzzSpans(size int, cuts []byte) []Span {
	var (
		spans []Span
		pos   int
	)

	for idx := 0; idx+1 < len(cuts); idx += 2 {
		start := pos + int(cuts[idx])%(size-pos+1)
		end := start + int(cuts[idx+1])%(size-start+1)
		spans = append(spans, Span{Start: start, End: end, IsContent: cuts[idx]&1 == 1})
		pos = end
	}

	return spans
}

func FuzzTile(f *testing.F) {
	seedSource := []byte("// head\n\nimport a\n\n\nfunc b\n\nfunc c\n")
	f.Add(seedSource, []byte{3, 8, 4, 6, 5, 6}, uint64(5))
	f.Add([]byte("\r\n\r\nx"), []byte{0, 0, 2, 1}, uint64(2))
	f.Add([]byte(""), []byte{1, 1}, uint64(1))

	f.Fuzz(func(t *testing.T, source, cuts []byte, mask uint64) {
		spans := fuzzSpans(len(source), cuts)
		hasContent := slices.ContainsFunc(spans, func(span Span) bool { return span.IsContent })

		all := Tile(source, spans, func(int) bool { return true })
		if hasContent && !bytes.Equal(all, source) {
			t.Fatalf("keep all changed the source: %q vs %q", all, source)
		}

		if !hasContent && all != nil {
			t.Fatalf("no content span still wrote %q", all)
		}

		keep := func(index int) bool { return mask>>(uint(index)%64)&1 == 1 }
		got := Tile(source, spans, keep)
		want := referenceTile(source, spans, keep)

		if !bytes.Equal(got, want) || (got == nil) != (want == nil) {
			t.Fatalf("got %q, want %q (source %q, spans %v)", got, want, source, spans)
		}

		keptContent := false
		for index, span := range spans {
			keptContent = keptContent || (span.IsContent && keep(index))
		}

		if keptContent && got == nil {
			t.Fatalf("a content span was kept but nothing was written (%v)", spans)
		}
	})
}

func BenchmarkTile(b *testing.B) {
	const unit = "func name() {}\n\n"

	source := []byte(strings.Repeat(unit, 1<<20/len(unit)))
	spans := make([]Span, 0, len(source)/len(unit))

	for start := 0; start+len(unit) <= len(source); start += len(unit) {
		spans = append(spans, Span{Start: start, End: start + len(unit) - 2, IsContent: true})
	}

	keep := func(index int) bool { return index%2 == 0 }

	b.SetBytes(int64(len(source)))
	b.ReportAllocs()

	for range b.N {
		Tile(source, spans, keep)
	}
}
