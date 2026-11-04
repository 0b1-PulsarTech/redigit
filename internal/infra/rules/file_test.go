package rules

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDecode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		file    string
		wantErr error
	}{
		{"the architecture example", "full.json", nil},
		{"an unknown top-level key", "unknown_key.json", ErrMalformed},
		{"malformed json", "malformed.json", ErrMalformed},
		{"a rule without a type", "missing_type.json", ErrMissingType},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			content, err := os.ReadFile(filepath.Join("testdata", testCase.file))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}

			_, err = Decode(content)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Decode() err = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestDecodeKeepsEachSection(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile(filepath.Join("testdata", "full.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	file, err := Decode(content)
	if err != nil {
		t.Fatalf("Decode() = %v", err)
	}

	if got := string(file.Languages["go"]["tags"]); got != `["integration"]` {
		t.Fatalf("go tags = %s", got)
	}

	if got := string(file.Languages["web"]["virtual"]); got != `{"virtual:app": "web/app.dart"}` {
		t.Fatalf("web virtual = %s", got)
	}

	if len(file.Rules) != 3 || file.Rules[1].Type != "go-router" {
		t.Fatalf("rules = %+v", file.Rules)
	}

	for name, raw := range map[string][]byte{
		"slice": file.Slice, "passes": file.Passes, "format": file.Format,
		"write": file.Write, "verify": file.Verify, "lineage": file.Lineage, "lint": file.Lint,
		"rule": file.Rules[0].Raw,
	} {
		if len(raw) == 0 {
			t.Fatalf("section %s was dropped", name)
		}
	}
}

func TestDecodeRefusesTrailingData(t *testing.T) {
	t.Parallel()

	_, err := Decode([]byte(`{} {}`))
	if !errors.Is(err, ErrMalformed) {
		t.Fatalf("Decode() err = %v, want ErrMalformed", err)
	}
}

func TestDecodeMessages(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		content string
		wantErr error
		want    string
	}{
		{"no bytes", ``, ErrEmpty, `empty file`},
		{"whitespace only", " \n\t ", ErrEmpty, `empty file`},
		{"a top-level null", `null`, ErrMalformed, `expected a JSON object`},
		{"a top-level array", `[]`, ErrMalformed, `expected a JSON object`},
		{"a top-level number", `3`, ErrMalformed, `expected a JSON object`},
		{"a truncated object", `{"languages":`, ErrMalformed, `invalid JSON at offset 13`},
		{"data after the object", `{} {}`, ErrMalformed, `invalid JSON at offset 4`},
		{
			"the first unknown field in order", `{"zed":1,"bar":2}`, ErrMalformed,
			`unknown field "bar"`,
		},
		{
			"the first bad language in order", `{"languages":{"zed":1,"bar":2}}`, ErrMalformed,
			`languages.bar: expected an object`,
		},
		{"null languages", `{"languages":null}`, ErrMalformed, `languages: expected an object`},
		{"array languages", `{"languages":[]}`, ErrMalformed, `languages: expected an object`},
		{
			"a null language", `{"languages":{"go":null}}`, ErrMalformed,
			`languages.go: expected an object`,
		},
		{
			"an array language", `{"languages":{"go":[]}}`, ErrMalformed,
			`languages.go: expected an object`,
		},
		{"null rules", `{"rules":null}`, ErrMalformed, `rules: expected an array`},
		{"an object for rules", `{"rules":{}}`, ErrMalformed, `rules: expected an array`},
		{"a rule that is no object", `{"rules":[7]}`, ErrMalformed, `rules[0]: expected an object`},
		{"a null rule", `{"rules":[null]}`, ErrMalformed, `rules[0]: expected an object`},
		{
			"a second rule without a type", `{"rules":[{"type":"a"},{"x":1}]}`,
			ErrMissingType, `rules[1]: missing type`,
		},
		{
			"a rule with an empty type", `{"rules":[{"type":""}]}`,
			ErrMissingType, `rules[0]: missing type`,
		},
		{
			"a type that is no string", `{"rules":[{"type":"a"},{"type":"b"},{"type":5}]}`,
			ErrMalformed, `rules[2].type: expected a string`,
		},
		{
			"a null type", `{"rules":[{"type":null}]}`, ErrMalformed,
			`rules[0].type: expected a string`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := Decode([]byte(testCase.content))
			if !errors.Is(err, testCase.wantErr) || !errors.Is(err, ErrMalformed) {
				t.Fatalf("Decode() err = %v, want ErrMalformed and %v", err, testCase.wantErr)
			}

			if err.Error() != testCase.want {
				t.Fatalf("Decode() err = %q, want %q", err, testCase.want)
			}
		})
	}
}

func TestDecodeKeepsLanguageSectionsRaw(t *testing.T) {
	t.Parallel()

	file, err := Decode([]byte(`{"languages":{"go":{"tagz":1,"tags":null},"web":{}}}`))
	if err != nil {
		t.Fatalf("Decode() = %v", err)
	}

	if got := string(file.Languages["go"]["tagz"]); got != "1" {
		t.Fatalf("tagz = %q, want the raw 1", got)
	}

	if got := string(file.Languages["go"]["tags"]); got != "null" {
		t.Fatalf("tags = %q, want the raw null left to the setting that owns it", got)
	}

	if web, found := file.Languages["web"]; !found || len(web) != 0 {
		t.Fatalf("web = %v (found %v), want an empty section", web, found)
	}
}

func TestDecodeAcceptsABOM(t *testing.T) {
	t.Parallel()

	file, err := Decode([]byte("\xEF\xBB\xBF" + `{"slice":1}`))
	if err != nil {
		t.Fatalf("Decode() = %v", err)
	}

	if got := string(file.Slice); got != "1" {
		t.Fatalf("slice = %q, want the raw 1", got)
	}
}

func TestDecodeReportsTheFirstBadLanguageInOrder(t *testing.T) {
	t.Parallel()

	for range 64 {
		_, err := Decode([]byte(`{"languages":{"zed":1,"bar":2}}`))
		if err == nil || err.Error() != `languages.bar: expected an object` {
			t.Fatalf("Decode() err = %v, want the first bad language, bar", err)
		}
	}
}

func TestDecodeMessagesOfBytes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		content string
		wantErr error
		want    string
	}{
		{
			"data after leading blanks, offset is 1-based",
			"\n\n   {} x",
			ErrMalformed,
			`invalid JSON at offset 9`,
		},
		{"data after a BOM", "\xEF\xBB\xBF{} x", ErrMalformed, `invalid JSON at offset 7`},
		{"a BOM then blanks", "\xEF\xBB\xBF\n {} x", ErrMalformed, `invalid JSON at offset 9`},
		{"a BOM alone", "\xEF\xBB\xBF", ErrEmpty, `empty file`},
		{
			"a byte that is no UTF-8", "{\"slice\":\"\xff\"}", ErrMalformed,
			`not valid UTF-8 at offset 11`,
		},
		{
			"a BOM then a byte that is no UTF-8", "\xEF\xBB\xBF{\"slice\":\"\xff\"}", ErrMalformed,
			`not valid UTF-8 at offset 14`,
		},
		{
			"a truncated multi-byte rune", "{\"slice\":\"\xe2\x82\"}", ErrMalformed,
			`not valid UTF-8 at offset 11`,
		},
		{"an unknown field", `{"foo":1}`, ErrMalformed, `unknown field "foo"`},
		{"a no-break space", "\xC2\xA0{}", ErrMalformed, `invalid JSON at offset 2`},
		{"a vertical tab", "\v{}", ErrMalformed, `invalid JSON at offset 1`},
		{"a BOM then a vertical tab", "\xEF\xBB\xBF\v{}", ErrMalformed, `invalid JSON at offset 4`},
		{"a trailing no-break space", "{}\xC2\xA0", ErrMalformed, `invalid JSON at offset 4`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := Decode([]byte(testCase.content))
			if !errors.Is(err, testCase.wantErr) || err.Error() != testCase.want {
				t.Fatalf("Decode() err = %v, want %v as %q", err, testCase.wantErr, testCase.want)
			}
		})
	}
}
