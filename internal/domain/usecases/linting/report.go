package linting

const (
	MeasureSourceAdded    = "source_added_over"
	MeasureGeneratedMixed = "generated_mixed"
	MeasureRowAdded       = "rows_over"
)

type (
	File struct {
		Path  string
		Added int
		Kind  Kind
	}

	Row struct {
		Position int
		Subject  string
		Files    []File
	}

	Finding struct {
		Measure  string
		Position int
		Subject  string
		Value    int
		Limit    int
	}

	Measure struct {
		Name       string
		Definition string
		Limit      int
		Count      int
		Excluded   []string
	}

	Report struct {
		Rows     int
		Findings []Finding
		Measures []Measure
	}
)

func Deferred() []string {
	return []string{
		"errcheck_pairs_split", "shells_without_tail", "shells_born_empty",
		"unused_at_birth", "born_as_stubs_gap_over_20_rows",
	}
}
