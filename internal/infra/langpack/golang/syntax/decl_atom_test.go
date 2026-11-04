package syntax

import "testing"

func TestAtomLabelNamesTheFileAndThePhysicalPosition(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		source string
		want   map[string]string
	}{
		{
			name:   "a block is labelled where it opens",
			source: "package sample\n\nconst (\n\tOne = iota\n\tTwo\n)\n",
			want: map[string]string{
				"One": "const-group:sample.go:3:1",
				"Two": "const-group:sample.go:3:1",
			},
		},
		{
			name:   "a spec is labelled where its first name sits",
			source: "package sample\n\nconst KeyA, KeyB = 1, 2\n\nvar VarA, VarB = 1, 2\n",
			want: map[string]string{
				"KeyA": "spec-group:sample.go:3:7",
				"VarB": "spec-group:sample.go:5:5",
			},
		},
		{
			name: "two blocks under one line directive keep different labels",
			source: `package sample

//line same.y:1
const (
	One = iota
	Two
)

//line same.y:1
const (
	Red = iota
	Green
)
`,
			want: map[string]string{
				"One": "const-group:sample.go:4:1",
				"Red": "const-group:sample.go:10:1",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			atomOf := atomsByName(t, testCase.source)

			for member, want := range testCase.want {
				if atomOf[member] != want {
					t.Fatalf("%s atom = %q, want %q", member, atomOf[member], want)
				}
			}
		})
	}
}
