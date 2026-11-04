package render

import (
	"errors"
	"slices"
	"testing"
)

func TestNeeded(t *testing.T) {
	t.Parallel()

	const header = "package p\n\nimport (\n\t\"bytes\"\n\tstr \"strings\"\n\t\"math/rand/v2\"\n" +
		"\t_ \"embed\"\n\t. \"fmt\"\n\t\"C\"\n)\n"

	testCases := []struct {
		name   string
		source string
		text   string
		want   []string
	}{
		{
			name: "an import the text uses", source: header,
			text: "func F() { _ = bytes.NewBuffer(nil) }", want: []string{`"bytes"`},
		},
		{
			name: "an alias is carried with its spec", source: header,
			text: "func F() string { return str.ToUpper(\"x\") }", want: []string{`str "strings"`},
		},
		{
			name: "a major version path is reached by the name before it", source: header,
			text: "func F() int { return rand.Int() }", want: []string{`"math/rand/v2"`},
		},
		{
			name:   "a major version path is reached by the version element",
			source: "package p\n\nimport \"example.com/fx/v1\"\n",
			text:   "func F() { v1.Run() }", want: []string{`"example.com/fx/v1"`},
		},
		{
			name: "two imports in the order of the file", source: header,
			text: "func F() { _ = rand.Int(); _ = bytes.Equal(nil, nil) }",
			want: []string{`"bytes"`, `"math/rand/v2"`},
		},
		{
			name: "a spec of a var group is read as a var", source: header,
			text: "Two = str.ToUpper(\"x\")", want: []string{`str "strings"`},
		},
		{
			name: "a spec of a type group is read as a type", source: header,
			text: "Box struct{ B bytes.Buffer }", want: []string{`"bytes"`},
		},
		{
			name: "a local binding is not a package", source: header,
			text: "func F(bytes []byte) int { return bytes.Len }",
		},
		{name: "cgo is never carried", source: header, text: "func F() { C.puts(nil) }"},
		{name: "a dot import is never carried", source: header, text: "func F() { Println() }"},
		{
			name: "a package the parent does not import", source: header,
			text: "func F() { _ = os.Args }",
		},
		{name: "text with no qualifier", source: header, text: "func F() int { return 1 }"},
		{name: "text that does not parse", source: header, text: "func ("},
		{name: "a source that does not parse", source: "package", text: "func F() { bytes.X() }"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := Needed(
				[]byte(testCase.source),
				testCase.text,
			); !slices.Equal(
				got,
				testCase.want,
			) {
				t.Fatalf("Needed() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestAdmit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		source  string
		specs   []string
		want    string
		wantErr error
	}{
		{
			name:   "no import yet puts the block under the package clause",
			source: "package p\n\nfunc F() {}\n", specs: []string{`"bytes"`},
			want: "package p\n\nimport (\n\t\"bytes\"\n)\n\nfunc F() {}\n",
		},
		{
			name:   "the block follows the last import",
			source: "package p\n\nimport \"strings\"\n\nfunc F() {}\n",
			specs:  []string{`"bytes"`, `str "os"`},
			want: "package p\n\nimport \"strings\"\n\nimport (\n\t\"bytes\"\n\tstr \"os\"\n)\n\n" +
				"func F() {}\n",
		},
		{
			name:   "a spec the file already holds is not added again",
			source: "package p\n\nimport \"bytes\"\n\nfunc F() {}\n",
			specs:  []string{`"bytes"`, `"bytes"`},
			want:   "package p\n\nimport \"bytes\"\n\nfunc F() {}\n",
		},
		{
			name:   "the same path under another name is added",
			source: "package p\n\nimport str \"strings\"\n\nfunc F() {}\n",
			specs:  []string{`"strings"`},
			want:   "package p\n\nimport str \"strings\"\n\nimport (\n\t\"strings\"\n)\n\nfunc F() {}\n",
		},
		{
			name:   "another path with the same name is added and left to the compiler",
			source: "package p\n\nimport \"crypto/rand\"\n\nfunc F() {}\n",
			specs:  []string{`"math/rand"`},
			want:   "package p\n\nimport \"crypto/rand\"\n\nimport (\n\t\"math/rand\"\n)\n\nfunc F() {}\n",
		},
		{name: "no spec", source: "package p\n", want: "package p\n"},
		{
			name: "a spec that does not parse", source: "package p\n", specs: []string{"=="},
			wantErr: errSpec,
		},
		{
			name:    "a source that does not parse",
			source:  "package",
			specs:   []string{`"os"`},
			wantErr: errSource,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := Admit([]byte(testCase.source), testCase.specs)
			if !errors.Is(err, testCase.wantErr) || string(got) != testCase.want {
				t.Fatalf("Admit() = %q, %v, want %q, %v", got, err, testCase.want, testCase.wantErr)
			}
		})
	}
}
