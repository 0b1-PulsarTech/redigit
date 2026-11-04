package format

import (
	"testing"
)

func TestToolsCovers(t *testing.T) {
	t.Parallel()

	fix := stage(t, []string{`{"position":1}`}, nil, fakeConfig)

	set, err := loadTools(fix.config)
	if err != nil {
		t.Fatalf("loadTools() err = %v", err)
	}

	testCases := []struct {
		name string
		path string
		want bool
	}{
		{name: "a configured extension", path: "pkg/a.go", want: true},
		{name: "an unconfigured extension", path: "README.md", want: false},
		{name: "no extension", path: "Makefile", want: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := set.covers(testCase.path); got != testCase.want {
				t.Fatalf("covers(%q) = %v, want %v", testCase.path, got, testCase.want)
			}
		})
	}
}

func TestLoadToolsRefusesABadSection(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		config string
	}{
		{name: "an unknown member", config: `{"format":{"nope":1}}`},
		{name: "a claimed twice extension", config: `{"format":{"languages":[` +
			`{"name":"a","extensions":[".go"]},{"name":"b","extensions":[".go"]}]}}`},
		{name: "a missing file", config: ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fix := stage(t, []string{`{"position":1}`}, nil, testCase.config)
			if testCase.config == "" {
				fix.config += ".gone"
			}

			if _, err := loadTools(fix.config); err == nil {
				t.Fatal("loadTools() accepted the config")
			}
		})
	}
}
