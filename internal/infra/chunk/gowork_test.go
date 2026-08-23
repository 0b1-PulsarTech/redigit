package chunk

import (
	"context"
	"testing"
)

func TestGoWorkHandles(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		path string
		want bool
	}{
		{"root go.work", "go.work", true},
		{"nested go.work", "sub/go.work", true},
		{"not go.work", "go.work.sum", false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := (GoWork{}).Handles(testCase.path); got != testCase.want {
				t.Fatalf("Handles(%q) = %v, want %v", testCase.path, got, testCase.want)
			}
		})
	}
}

func TestGoWorkChunks(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		source    string
		wantNames []string
	}{
		{"empty file", "", []string{fileDeclName}},
		{"header only", "go 1.26\n", []string{fileDeclName}},
		{
			"single use line",
			"go 1.26\n\nuse ./modules/lead\n",
			[]string{fileDeclName, "use ./modules/lead"},
		},
		{
			"use block with a commented-out line",
			"go 1.26\n\nuse (\n\t./modules/lead\n\t// ./modules/nothere\n\t./modules/contact\n)\n",
			[]string{fileDeclName, "use ./modules/lead", "use ./modules/contact"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			decls, err := (GoWork{}).Chunks(
				context.Background(),
				"go.work",
				[]byte(testCase.source),
			)
			if err != nil {
				t.Fatalf("Chunks() error = %v", err)
			}

			assertReconstructs(t, testCase.source, decls)
			assertDeclNames(t, decls, testCase.wantNames)
		})
	}
}
