package linting

type Limits struct {
	SourceAdded    int
	GeneratedMixed int
	RowAdded       int
	StubGap        int
}

func DefaultLimits() Limits {
	// These are the finish-cap numbers the original scanner used.
	return Limits{
		SourceAdded: 900, GeneratedMixed: 900, RowAdded: 850, //nolint:mnd // see above
		StubGap: 20, //nolint:mnd // the gap the original scanner called a stub
	}
}
