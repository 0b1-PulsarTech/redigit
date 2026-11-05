package anchorstate

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestIsTemp(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		file string
		want bool
	}{
		{name: "what CreateTemp makes", file: ".redigit-tmp-123456789", want: true},
		{name: "a single digit", file: ".redigit-tmp-0", want: true},
		{name: "the user's name", file: ".redigit-tmp-mine"},
		{name: "digits then letters", file: ".redigit-tmp-12a"},
		{name: "no digits at all", file: ".redigit-tmp-"},
		{name: "another prefix", file: ".redigit-tmpx-12"},
		{name: "a longer name", file: "x.redigit-tmp-12"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := isTemp(testCase.file); got != testCase.want {
				t.Fatalf("isTemp(%q) = %v, want %v", testCase.file, got, testCase.want)
			}
		})
	}
}

func TestReservedIn(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		stageName + "/x": "s", oldName + "/y": "o", tmpPrefix + "42": "t", DerivedFile: "{}",
		tmpPrefix + "mine": "m", stageName + "-mine": "m", oldName + "-mine/z": "m",
		".redigit-layers.json": "{}", "notes.txt": "n",
	})

	got, err := reservedIn(dir)
	slices.Sort(got)

	want := []string{oldName, stageName, tmpPrefix + "42"}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("reservedIn() = %q, %v, want %q", got, err, want)
	}
}

func TestClearTree(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		files    map[string]string
		dirs     []string
		links    map[string]string
		wantErr  error
		wantKept []string
		wantGone bool
	}{
		{name: "nothing there", wantGone: true},
		{name: "a file", files: map[string]string{"t": "f"}, wantGone: true},
		{name: "a tree with no marker and nothing in it goes", dirs: []string{"t"}, wantGone: true},
		{
			name:     "a tree with no marker keeps what is in it and says so",
			files:    map[string]string{"t/a/b": "x", "t/c": "y"},
			wantErr:  ErrForeign,
			wantKept: []string{"t/a/b", "t/c"},
		},
		{
			name:     "a marker that is a directory is no marker",
			files:    map[string]string{"t/" + markerFile + "/x": "x"},
			wantErr:  ErrForeign,
			wantKept: []string{"t/" + markerFile + "/x"},
		},
		{
			name:     "a marker that is a link is no marker",
			files:    map[string]string{"outside/x": "x"},
			dirs:     []string{"t"},
			links:    map[string]string{"t/" + markerFile: "../outside"},
			wantErr:  ErrForeign,
			wantKept: []string{"t/" + markerFile, "outside/x"},
		},
		{
			name:     "a marked tree goes by its list",
			files:    map[string]string{"t/" + markerFile: "a/b\n", "t/a/b": "x"},
			wantGone: true,
		},
		{
			name:     "a marked tree whose texts are gone goes with its marker",
			files:    map[string]string{"t/" + markerFile: "a/b\n"},
			wantGone: true,
		},
		{
			name: "a marked tree keeps what it did not list, and its marker, and says so",
			files: map[string]string{
				"t/" + markerFile: "a/b\n", "t/a/b": "x", "t/a/mine": "m", "t/note": "n",
			},
			wantErr:  ErrForeign,
			wantKept: []string{"t/a/mine", "t/note", "t/" + markerFile},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			base := t.TempDir()
			writeFiles(t, base, testCase.files)
			writeLayout(t, base, testCase.dirs, testCase.links)

			err := clearTree(filepath.Join(base, "t"))
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("clearTree() = %v, want %v", err, testCase.wantErr)
			}

			_, statErr := os.Lstat(filepath.Join(base, "t"))
			if (statErr != nil) != testCase.wantGone {
				t.Fatalf("tree gone = %v, want %v", statErr != nil, testCase.wantGone)
			}

			for _, name := range testCase.wantKept {
				if _, statErr := os.Stat(filepath.Join(base, name)); statErr != nil {
					t.Fatalf("%s did not survive: %v", name, statErr)
				}
			}
		})
	}
}

func TestClearOwned(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		files       map[string]string
		locked      string
		names       []string
		wantErr     bool
		wantForeign bool
		wantKept    []string
		wantGone    []string
	}{
		{
			name: "a temporary file goes", files: map[string]string{tmpPrefix + "7": "t"},
			names: []string{tmpPrefix + "7"}, wantGone: []string{tmpPrefix + "7"},
		},
		{
			name:  "a temporary directory with a file in it is kept and named",
			files: map[string]string{tmpPrefix + "7/x": "mine"}, names: []string{tmpPrefix + "7"},
			wantErr: true, wantForeign: true, wantKept: []string{tmpPrefix + "7/x"},
		},
		{
			name:  "a derived config that is a directory with a file in it is kept and named",
			files: map[string]string{DerivedFile + "/x": "mine"}, names: []string{DerivedFile},
			wantErr: true, wantForeign: true, wantKept: []string{DerivedFile + "/x"},
		},
		{
			name: "a failure beside a foreign file is the failure and not the foreign file",
			files: map[string]string{
				oldName + "/" + markerFile: "", oldName + "/mine": "m",
				stageName + "/" + markerFile: "z/t\n", stageName + "/z/t": "t",
			},
			locked: stageName + "/z", names: []string{oldName, stageName},
			wantErr: true, wantKept: []string{oldName + "/mine", stageName + "/z/t"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if testCase.locked != "" && os.Geteuid() == 0 {
				t.Skip("root ignores directory permissions")
			}

			dir := t.TempDir()
			writeFiles(t, dir, testCase.files)

			if testCase.locked != "" {
				lock(t, filepath.Join(dir, testCase.locked))
			}

			err := clearOwned(dir, testCase.names)

			isForeign := errors.Is(err, ErrForeign)
			if (err != nil) != testCase.wantErr || isForeign != testCase.wantForeign {
				t.Fatalf("clearOwned() = %v, want err %v, foreign %v",
					err, testCase.wantErr, testCase.wantForeign)
			}

			for _, name := range testCase.wantKept {
				if _, statErr := os.Stat(filepath.Join(dir, name)); statErr != nil {
					t.Fatalf("%s did not survive: %v", name, statErr)
				}
			}

			for _, name := range testCase.wantGone {
				if _, statErr := os.Lstat(filepath.Join(dir, name)); statErr == nil {
					t.Fatalf("%s survived", name)
				}
			}
		})
	}
}

func TestReservedNamesKeepWhatIsNotRedigitsOnEveryPass(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		files      map[string]string
		kept       string
		wantMarker string
		isSave     bool
	}{
		{
			name:  "a save over the old conflicts",
			files: map[string]string{oldName + "/" + markerFile: "a/b\n", oldName + "/mine": "m"},
			kept:  oldName + "/mine", wantMarker: oldName + "/" + markerFile, isSave: true,
		},
		{
			name:  "an abort over the old conflicts",
			files: map[string]string{oldName + "/" + markerFile: "a/b\n", oldName + "/mine": "m"},
			kept:  oldName + "/mine", wantMarker: oldName + "/" + markerFile,
		},
		{
			name: "a save over a stage",
			files: map[string]string{
				stageName + "/" + markerFile: "a/b\n", stageName + "/mine": "m",
			},
			kept: stageName + "/mine", wantMarker: stageName + "/" + markerFile, isSave: true,
		},
		{
			name: "an abort over a stage",
			files: map[string]string{
				stageName + "/" + markerFile: "a/b\n", stageName + "/mine": "m",
			},
			kept: stageName + "/mine", wantMarker: stageName + "/" + markerFile,
		},
		{
			name:  "a save over a temporary directory",
			files: map[string]string{tmpPrefix + "9/mine": "m"}, kept: tmpPrefix + "9/mine",
			isSave: true,
		},
		{
			name:  "an abort over a temporary directory",
			files: map[string]string{tmpPrefix + "9/mine": "m"}, kept: tmpPrefix + "9/mine",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := filepath.Join(t.TempDir(), "state")
			if err := Save(dir, stateOf("", "aaa111")); err != nil {
				t.Fatalf("Save() = %v", err)
			}

			writeFiles(t, dir, testCase.files)

			for pass := 1; pass <= 2; pass++ {
				var err error
				if testCase.isSave {
					err = Save(dir, stateOf("", "bbb222"))
					if !errors.Is(err, ErrForeign) {
						t.Fatalf("pass %d: Save() = %v, want ErrForeign", pass, err)
					}
				} else if err = Remove(dir); err != nil {
					t.Fatalf("pass %d: Remove() = %v", pass, err)
				}

				for _, name := range []string{testCase.kept, testCase.wantMarker} {
					if _, statErr := os.Stat(filepath.Join(dir, name)); statErr != nil {
						t.Fatalf("pass %d: %s did not survive: %v", pass, name, statErr)
					}
				}
			}
		})
	}
}
