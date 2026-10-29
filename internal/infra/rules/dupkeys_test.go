package rules

import (
	"errors"
	"strings"
	"testing"
)

func TestDecodeRefusesDuplicateKeys(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		content string
		want    string
	}{
		{"the top level", `{"rules":[],"rules":[]}`, `duplicate key "rules" at offset 19`},
		{
			"a language",
			`{"languages":{"go":{"tags":["a"]},"go":{"tags":["b"]}}}`,
			`duplicate key "go" at offset 38`,
		},
		{
			"a setting",
			`{"languages":{"go":{"tags":["a"],"tags":["b"]}}}`,
			`duplicate key "tags" at offset 39`,
		},
		{
			"a virtual module",
			`{"languages":{"web":{"virtual":{"a":"x","a":"y"}}}}`,
			`duplicate key "a" at offset 43`,
		},
		{
			"inside a rule",
			`{"rules":[{"type":"t","x":{"k":1,"k":2}}]}`,
			`duplicate key "k" at offset 36`,
		},
		{
			"a rule type repeated",
			`{"rules":[{"type":"a","type":"b"}]}`,
			`duplicate key "type" at offset 28`,
		},
		{
			"after a BOM and blanks", "\xEF\xBB\xBF \n{\"a\":1,\"a\":2}",
			`duplicate key "a" at offset 15`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := Decode([]byte(testCase.content))
			if !errors.Is(err, ErrDuplicateKey) || !errors.Is(err, ErrMalformed) {
				t.Fatalf("Decode() err = %v, want ErrDuplicateKey and ErrMalformed", err)
			}

			if err.Error() != testCase.want {
				t.Fatalf("Decode() err = %q, want %q", err, testCase.want)
			}
		})
	}
}

func TestDecodeAcceptsRepeatedKeysAcrossScopes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		content string
	}{
		{"sibling objects", `{"languages":{"go":{"tags":["a"]},"web":{"tags":["b"]}}}`},
		{"array elements", `{"rules":[{"type":"a","x":1},{"type":"a","x":1}]}`},
		{"a key and a value of the same text", `{"rules":[{"type":"type"}]}`},
		{"nested objects under one name", `{"slice":{"a":{"a":1}}}`},
		{"repeated strings in an array", `{"languages":{"go":{"tags":["a","b","a"]}}}`},
		{"an object after strings in an array", `{"slice":["a",{"a":1},"a"]}`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if _, err := Decode([]byte(testCase.content)); err != nil {
				t.Fatalf("Decode() err = %v, want none", err)
			}
		})
	}
}

func TestDecodeDeepNestingFailsFast(t *testing.T) {
	t.Parallel()

	const depth = 100000

	for _, open := range []string{"[", `{"a":`} {
		content := strings.Repeat(open, depth) + "1"

		if _, err := Decode([]byte(content)); !errors.Is(err, ErrMalformed) {
			t.Fatalf("Decode(%q nested) err = %v, want ErrMalformed", open, err)
		}
	}
}
