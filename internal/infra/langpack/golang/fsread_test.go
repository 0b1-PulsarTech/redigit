package golang

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestReadSource(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, root, "a.go", "package a\n")

	testCases := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"existing file", "a.go", false},
		{"missing file", "missing.go", true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := ReadSource(entities.Snapshot{Root: root}, testCase.path)
			if (err != nil) != testCase.wantErr {
				t.Fatalf("readSource() error = %v, wantErr %v", err, testCase.wantErr)
			}
		})
	}
}

func TestSliceSpan(t *testing.T) {
	t.Parallel()

	source := []byte("package a\n\nfunc F() {}\n")

	testCases := []struct {
		name string
		span entities.Span
		want string
	}{
		{"valid span", entities.Span{Start: 0, End: 7}, "package"},
		{"start past end is invalid", entities.Span{Start: 5, End: 2}, ""},
		{"end past source length is invalid", entities.Span{Start: 0, End: len(source) + 10}, ""},
		{"negative start is invalid", entities.Span{Start: -1, End: 5}, ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := SliceSpan(source, testCase.span); got != testCase.want {
				t.Fatalf("sliceSpan() = %q, want %q", got, testCase.want)
			}
		})
	}
}
