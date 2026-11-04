package format

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/transport/cli/manifest"
)

func TestFinalSpots(t *testing.T) {
	t.Parallel()

	blob := func(path, handle string) []manifest.Blob {
		return []manifest.Blob{{Path: path, Mode: "100644", Handle: handle}}
	}

	testCases := []struct {
		name string
		rows []manifest.Row
		want map[spot]struct{}
	}{
		{
			name: "the highest position wins, wherever its row sits",
			rows: []manifest.Row{
				{Position: 3, Blobs: blob("a.go", "h3")},
				{Position: 1, Blobs: blob("a.go", "h1")},
			},
			want: map[spot]struct{}{{0, 0}: {}},
		},
		{
			name: "each path has its own final",
			rows: []manifest.Row{
				{Position: 1, Blobs: blob("a.go", "h1")},
				{Position: 2, Blobs: blob("b.go", "h2")},
				{Position: 3, Blobs: blob("a.go", "h3")},
			},
			want: map[spot]struct{}{{1, 0}: {}, {2, 0}: {}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := finalSpots(testCase.rows)
			if len(got) != len(testCase.want) {
				t.Fatalf("finalSpots() = %v, want %v", got, testCase.want)
			}

			for want := range testCase.want {
				if _, found := got[want]; !found {
					t.Fatalf("finalSpots() = %v, want %v", got, testCase.want)
				}
			}
		})
	}
}

func TestRegular(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		blob manifest.Blob
		want bool
	}{
		{name: "a file", blob: manifest.Blob{Mode: "100644", Handle: "h"}, want: true},
		{name: "an executable", blob: manifest.Blob{Mode: "100755", Handle: "h"}, want: true},
		{name: "a symlink", blob: manifest.Blob{Mode: "120000", Handle: "h"}, want: false},
		{name: "no handle", blob: manifest.Blob{Mode: "100644"}, want: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := regular(testCase.blob); got != testCase.want {
				t.Fatalf("regular() = %v, want %v", got, testCase.want)
			}
		})
	}
}
