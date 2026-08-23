package entities

import "testing"

// span builds the inclusive run of positions from first to last, which is what
// most slices of a history are before any merging pulls distant pieces
// together.
func span(first, last Position) []Position {
	at := make([]Position, 0, last-first+1)
	for position := first; position <= last; position++ {
		at = append(at, position)
	}

	return at
}

func TestFeaturesIndexAt(t *testing.T) {
	t.Parallel()

	// Given out of order on purpose: NewFeatures sorts, and a caller reading
	// a file has no reason to have sorted first.
	features := NewFeatures([]Feature{
		{Name: "agendamento", At: span(243, 245)},
		{Name: "bootstrap", At: span(0, 12)},
	})

	testCases := []struct {
		name string
		at   Position
		want string
	}{
		{name: "the boundary is inclusive at the start", at: 0, want: "bootstrap"},
		{name: "the boundary is inclusive at the end", at: 12, want: "bootstrap"},
		{name: "one past a slice belongs to no slice", at: 13, want: ""},
		{name: "a later slice after sorting", at: 244, want: "agendamento"},
	}

	all := features.All()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := ""
			if index := features.IndexAt(testCase.at); index >= 0 {
				got = all[index].Name
			}

			if got != testCase.want {
				t.Fatalf("IndexAt(%d) names %q, want %q", testCase.at, got, testCase.want)
			}
		})
	}
}

func TestFeaturesOverlaps(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		features []Feature
		want     int
	}{
		{
			name:     "adjacent slices do not overlap",
			features: []Feature{{Name: "a", At: span(0, 12)}, {Name: "b", At: span(13, 20)}},
			want:     0,
		},
		{
			name:     "a shared boundary position is an overlap",
			features: []Feature{{Name: "a", At: span(0, 12)}, {Name: "b", At: span(12, 20)}},
			want:     1,
		},
		{
			name:     "one slice inside another",
			features: []Feature{{Name: "a", At: span(0, 100)}, {Name: "b", At: span(40, 50)}},
			want:     1,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := NewFeatures(testCase.features).Overlaps()
			if len(got) != testCase.want {
				t.Fatalf("Overlaps() found %d, want %d", len(got), testCase.want)
			}
		})
	}
}

func TestFeaturesEmpty(t *testing.T) {
	t.Parallel()

	if !(Features{}).Empty() {
		t.Fatal("the zero Features reports itself non-empty")
	}

	if NewFeatures([]Feature{{Name: "a", At: span(0, 1)}}).Empty() {
		t.Fatal("a Features holding one slice reports itself empty")
	}
}
