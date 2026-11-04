package selector

import "testing"

type (
	readerFake struct {
		reads map[string][]string
		calls map[string]int
	}

	removedCase struct {
		name     string
		reads    map[string][]string
		preimage string
		final    string
		method   string
		imports  []string
		want     bool
	}
)

func (fake *readerFake) Reads(_, text string) ([]string, bool) {
	fake.calls[text]++
	names, isParsed := fake.reads[text]

	return names, isParsed
}

func checkRemoved(t *testing.T, testCases []removedCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fake := &readerFake{reads: testCase.reads, calls: map[string]int{}}
			memo := NewMemo(fake)

			got := memo.Removed(
				"h.go", testCase.preimage, testCase.final, testCase.method, testCase.imports,
			)
			if got != testCase.want {
				t.Fatalf("Removed() = %t, want %t", got, testCase.want)
			}
		})
	}
}

func TestMemoRemoved(t *testing.T) {
	t.Parallel()

	packages := []string{`"example.com/p/entities"`, `_ "example.com/p/blank"`}

	testCases := []removedCase{
		{
			name:     "a selector both texts hold",
			reads:    map[string][]string{"pre": {".M"}, "fin": {".M"}},
			preimage: "pre", final: "fin", method: "M",
		},
		{
			name:     "a selector the final text lost",
			reads:    map[string][]string{"pre": {".M"}, "fin": {"x"}},
			preimage: "pre", final: "fin", method: "M", want: true,
		},
		{
			name:     "a free qualifier that is no import selects",
			reads:    map[string][]string{"pre": {"pkgvar.M"}, "fin": {}},
			preimage: "pre", final: "fin", method: "M", imports: packages, want: true,
		},
		{
			name:     "a package qualifier selects nothing",
			reads:    map[string][]string{"pre": {"entities.M"}, "fin": {}},
			preimage: "pre", final: "fin", method: "M", imports: packages,
		},
		{
			name:     "a blank import is no package",
			reads:    map[string][]string{"pre": {"blank.M"}, "fin": {}},
			preimage: "pre", final: "fin", method: "M", imports: packages, want: true,
		},
		{
			name:     "a free qualifier in the final text holds nothing",
			reads:    map[string][]string{"pre": {".M"}, "fin": {"hlp.M"}},
			preimage: "pre", final: "fin", method: "M", want: true,
		},
		{
			name:     "a package var called in both texts is a false fold",
			reads:    map[string][]string{"pre": {"pkgvar.M"}, "fin": {"pkgvar.M"}},
			preimage: "pre", final: "fin", method: "M", want: true,
		},
	}

	checkRemoved(t, testCases)
}

func TestMemoRemovedDeclinesWhatItCannotRead(t *testing.T) {
	t.Parallel()

	testCases := []removedCase{
		{
			name:     "a final text that does not parse holds nothing",
			reads:    map[string][]string{"pre": {".M"}},
			preimage: "pre", final: "fin", method: "M", want: true,
		},
		{
			name:     "a pre-image that does not parse searches both texts",
			reads:    map[string][]string{},
			preimage: "d.Key.Name", final: "d.Key.Name", method: "Name",
		},
		{
			name:     "an unparsed pre-image with a final text lacking it",
			reads:    map[string][]string{},
			preimage: "d.Key.Name", final: "d.Key", method: "Name", want: true,
		},
		{
			name:     "a name that is only a prefix",
			reads:    map[string][]string{"pre": {".Names"}, "fin": {}},
			preimage: "pre", final: "fin", method: "M",
		},
		{
			name:     "a name that is only a suffix",
			reads:    map[string][]string{"pre": {".XM"}, "fin": {}},
			preimage: "pre", final: "fin", method: "M",
		},
	}

	checkRemoved(t, testCases)
}

func TestMemoParsesEachTextOnce(t *testing.T) {
	t.Parallel()

	fake := &readerFake{
		reads: map[string][]string{"pre": {".M", ".N"}, "fin": {}},
		calls: map[string]int{},
	}
	memo := NewMemo(fake)

	for _, method := range []string{"M", "N", "M"} {
		memo.Removed("h.go", "pre", "fin", method, nil)
	}

	if fake.calls["pre"] != 1 || fake.calls["fin"] != 1 {
		t.Fatalf("calls = %v, want one per text", fake.calls)
	}
}
