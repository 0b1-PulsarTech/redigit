package anchorstate

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestReadMarker(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		files      map[string]string
		dirs       []string
		wantListed []string
		wantMarked bool
	}{
		{name: "no marker"},
		{name: "a marker that is a directory", dirs: []string{markerFile}},
		{name: "an empty marker", files: map[string]string{markerFile: ""}, wantMarked: true},
		{
			name:  "one path a line",
			files: map[string]string{markerFile: "aaa/x.final\nbbb/y.preimage\n"}, wantMarked: true,
			wantListed: []string{"aaa/x.final", "bbb/y.preimage"},
		},
		{
			name: "lines that name more than a rev and a file are skipped",
			files: map[string]string{
				markerFile: "../x\na/../b\n/abs\na\na/b/c\n./x\n./\n.\n..\na/.\na/..\n\na/ok\n",
			},
			wantMarked: true, wantListed: []string{"a/ok"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			writeFiles(t, dir, testCase.files)
			writeLayout(t, dir, testCase.dirs, nil)

			listed, hasMarker, err := readMarker(dir)
			if err != nil || hasMarker != testCase.wantMarked ||
				!slices.Equal(listed, testCase.wantListed) {
				t.Fatalf("readMarker() = %q, %v, %v, want %q, %v",
					listed, hasMarker, err, testCase.wantListed, testCase.wantMarked)
			}
		})
	}
}

func TestUnlisted(t *testing.T) {
	t.Parallel()

	listed := []string{"aaa/x.final"}

	testCases := []struct {
		name      string
		files     map[string]string
		dirs      []string
		links     map[string]string
		want      string
		wantEmpty bool
	}{
		{
			name:      "only what the marker lists",
			files:     map[string]string{markerFile: "", "aaa/x.final": "x"},
			wantEmpty: true,
		},
		{
			name:  "a file nobody listed",
			files: map[string]string{markerFile: "", "aaa/x.final": "x", "aaa/mine.go": "m"},
			want:  "aaa/mine.go",
		},
		{
			name:  "a directory nobody listed",
			files: map[string]string{markerFile: "", "aaa/x.final": "x", "mine/y": "m"},
			want:  "mine",
		},
		{
			name: "a listed name that is a link", files: map[string]string{markerFile: ""},
			dirs: []string{"aaa", "target"}, links: map[string]string{"aaa/x.final": "../target"},
			want: "aaa/x.final",
		},
		{
			name: "an empty directory nobody listed", files: map[string]string{markerFile: ""},
			dirs: []string{"hollow"}, want: "hollow",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			writeFiles(t, dir, testCase.files)
			writeLayout(t, dir, testCase.dirs, testCase.links)

			got, err := unlisted(dir, listed)
			if err != nil || got != testCase.want {
				t.Fatalf("unlisted() = %q, %v, want %q", got, err, testCase.want)
			}
		})
	}
}

func TestDropTextsKeepsWhatItDidNotWrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"aaa/x.final": "x", "aaa/mine.go": "m", "bbb/y.final": "y",
	})

	if err := dropTexts(dir, []string{"aaa/x.final", "bbb/y.final", "ccc/gone.final"}); err != nil {
		t.Fatalf("dropTexts() = %v", err)
	}

	for name, wantKept := range map[string]bool{
		"aaa/x.final": false, "aaa/mine.go": true, "aaa": true, "bbb/y.final": false, "bbb": false,
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); (err == nil) != wantKept {
			t.Fatalf("%s kept = %v, want %v", name, err == nil, wantKept)
		}
	}
}
