package selector

import "testing"

func TestRemoved(t *testing.T) {
	t.Parallel()

	packages := []string{`"example.com/p/entities"`, `_ "example.com/p/blank"`}

	testCases := []struct {
		name     string
		preimage string
		final    string
		method   string
		imports  []string
		want     bool
	}{
		{
			name:     "a selector both texts hold",
			preimage: "d.Key.Name", final: "d.Key.Name", method: "Name",
		},
		{
			name:     "a selector the final text lost",
			preimage: "d.Key.Name", final: "d.Key", method: "Name", want: true,
		},
		{
			name:     "a selector with an empty final text",
			preimage: "x.Name", method: "Name", want: true,
		},
		{
			name:     "a name that is only a prefix",
			preimage: "x.Names", final: "x", method: "Name",
		},
		{
			name:     "a package qualifier",
			preimage: "entities.Plan", method: "Plan", imports: packages,
		},
		{
			name:     "a package qualifier beside a call",
			preimage: "entities.Plan(a.Plan())", method: "Plan", imports: packages, want: true,
		},
		{
			name:     "a call on a result has no qualifier",
			preimage: "f().Plan", method: "Plan", want: true,
		},
		{
			name:     "a blank import is no qualifier",
			preimage: "f().Plan", method: "Plan", imports: packages, want: true,
		},
		{
			name:     "a final text holding the package form only",
			preimage: "x.Plan", final: "entities.Plan", method: "Plan",
			imports: packages, want: true,
		},
		{
			name:     "an accented neighbour is part of the name",
			preimage: "x.Plan\u00e9", final: "x", method: "Plan",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := (*Memo)(nil).Removed(
				"h.go", testCase.preimage, testCase.final, testCase.method, testCase.imports,
			)
			if got != testCase.want {
				t.Fatalf("Removed() = %t, want %t", got, testCase.want)
			}
		})
	}
}
