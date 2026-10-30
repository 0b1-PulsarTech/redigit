package slots

import (
	"errors"
	"slices"
	"testing"
)

func TestParseTree(t *testing.T) {
	t.Parallel()

	listing := "040000 tree t-lib\tlib\x00100644 blob b-mod\tlib/go.mod\x00" +
		"100644 blob b-root\tgo.mod\x00100644 blob b-pub\tapp/pubspec.yaml\x00" +
		"100644 blob b-doc\tdocs/go.mod.md\x00160000 commit c-sub\tvendor/go.mod\x00" +
		"040000 tree t-app\tapp\x00"
	markers := map[string]string{"go.mod": "go", "pubspec.yaml": "dart"}

	testCases := []struct {
		name      string
		listing   string
		wantFiles []markerFile
		wantTrees map[string]string
		wantErr   error
	}{
		{"empty", "", nil, map[string]string{}, nil},
		{
			"markers and subtrees",
			listing,
			[]markerFile{
				{dir: "lib", language: "go", blob: "b-mod"},
				{dir: ".", language: "go", blob: "b-root"},
				{dir: "app", language: "dart", blob: "b-pub"},
			},
			map[string]string{"lib": "t-lib", "app": "t-app"},
			nil,
		},
		{"an entry without a tab", "100644 blob b-mod go.mod\x00", nil, nil, ErrBadTree},
		{"an entry with a short header", "100644 b-mod\tgo.mod\x00", nil, nil, ErrBadTree},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			found, err := parseTree([]byte(testCase.listing), markers)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}

			if !slices.Equal(found.files, testCase.wantFiles) {
				t.Fatalf("files = %+v, want %+v", found.files, testCase.wantFiles)
			}

			for dir, oid := range testCase.wantTrees {
				if found.trees[dir] != oid {
					t.Fatalf("tree of %s = %q, want %q", dir, found.trees[dir], oid)
				}
			}
		})
	}
}

func TestParseTreeSkipsToolPaths(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		path string
		want bool
	}{
		{"a plain module", "real/go.mod", true},
		{"a nested module", "a/b/go.mod", true},
		{"a dir that only starts like testdata", "testdatax/go.mod", true},
		{"an underscore inside a name", "my_dir/go.mod", true},
		{"testdata at the root", "testdata/go.mod", false},
		{"testdata nested", "a/testdata/x/go.mod", false},
		{"vendor", "vendor/x/go.mod", false},
		{"vendor nested", "a/vendor/go.mod", false},
		{"an underscore dir", "_hidden/go.mod", false},
		{"a nested underscore dir", "a/b/_c/go.mod", false},
		{"a dot dir", ".hidden/go.mod", false},
		{"a nested dot dir", "a/.git-like/go.mod", false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			listing := "100644 blob b-oid\t" + testCase.path + "\x00"

			found, err := parseTree([]byte(listing), map[string]string{"go.mod": "go"})
			if err != nil {
				t.Fatalf("parseTree: %v", err)
			}

			if isListed := len(found.files) == 1; isListed != testCase.want {
				t.Fatalf("listed = %v, want %v", isListed, testCase.want)
			}
		})
	}
}

func TestParseTreeFlagsLinks(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		listing string
		want    bool
	}{
		{"no link", "100644 blob b1\ta/x.go\x00", false},
		{"a file link", "120000 blob b1\ta/x.go\x00", true},
		{"a link under testdata", "120000 blob b1\ta/testdata/x\x00", false},
		{"a link under a nested testdata", "120000 blob b1\ta/testdata/p/x\x00", false},
		{"a link under vendor", "120000 blob b1\tvendor/x\x00", true},
		{"a link under a nested vendor", "120000 blob b1\ta/vendor/p/x.go\x00", true},
		{"a link under an underscore dir", "120000 blob b1\ta/_gen/x.go\x00", true},
		{"a link under a dot dir", "120000 blob b1\ta/.gen/x.go\x00", true},
		{"a link named testdata", "120000 blob b1\ta/testdata\x00", true},
		{
			"a link under a dir that only starts like testdata",
			"120000 blob b1\ttestdatax/x\x00",
			true,
		},
		{"an executable file", "100755 blob b1\ta/x.sh\x00", false},
		{"a submodule named vendor", "160000 commit c1\tvendor\x00", true},
		{"a nested submodule named vendor", "160000 commit c1\ta/vendor\x00", true},
		{"a submodule with another name", "160000 commit c1\tdep\x00", false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			found, err := parseTree([]byte(testCase.listing), map[string]string{})
			if err != nil || found.hasLink != testCase.want {
				t.Fatalf("hasLink = %v err = %v, want %v", found.hasLink, err, testCase.want)
			}
		})
	}
}

func TestParseTreeCollectsOnlyBuildableGoSources(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		path string
		want int
	}{
		{"a plain source", "a/x.go", 1},
		{"a root source", "x.go", 1},
		{"an underscore basename", "a/_x.go", 0},
		{"a dot basename", "a/.x.go", 0},
		{"an underscore in the middle", "a/x_y.go", 1},
		{"a test source", "a/x_test.go", 1},
		{"not a go file", "a/x.txt", 0},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			found, err := parseTree([]byte("100644 blob b1\t"+testCase.path+"\x00"), nil)
			if err != nil || len(found.goSrc) != testCase.want {
				t.Fatalf("goSrc = %v err = %v, want %d", found.goSrc, err, testCase.want)
			}
		})
	}
}

func TestParseTreeCollectsWorkFiles(t *testing.T) {
	t.Parallel()

	listing := "100644 blob w-root\tgo.work\x00100644 blob s-root\tgo.work.sum\x00" +
		"100644 blob w-sub\tsub/go.work\x00100644 blob w-test\ttestdata/go.work\x00" +
		"100644 blob b-mod\tgo.mod\x00100644 blob b-other\tgo.workx\x00"
	want := []markerFile{
		{dir: ".", name: "go.work", blob: "w-root"},
		{dir: ".", name: "go.work.sum", blob: "s-root"},
		{dir: "sub", name: "go.work", blob: "w-sub"},
	}

	found, err := parseTree([]byte(listing), map[string]string{"go.mod": "go"})
	if err != nil {
		t.Fatalf("parseTree: %v", err)
	}

	if !slices.Equal(found.works, want) {
		t.Fatalf("works = %+v, want %+v", found.works, want)
	}
}

func TestParseBlobs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		out     string
		oids    []string
		want    []string
		wantErr error
	}{
		{"none", "", nil, []string{}, nil},
		{
			"two blobs in request order",
			"b1 blob 3\nabc\nb2 blob 4\nabcd\n",
			[]string{"b1", "b2"},
			[]string{"abc", "abcd"},
			nil,
		},
		{"a missing object", "b1 missing\n", []string{"b1"}, nil, ErrBadBlob},
		{"a truncated body", "b1 blob 9\nabc\n", []string{"b1"}, nil, ErrBadBlob},
		{"a garbled header", "nonsense\n", []string{"b1"}, nil, ErrBadBlob},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseBlobs([]byte(testCase.out), testCase.oids)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}

			if err == nil &&
				!slices.EqualFunc(got, testCase.want, func(left []byte, right string) bool {
					return string(left) == right
				}) {
				t.Fatalf("blobs = %q, want %q", got, testCase.want)
			}
		})
	}
}
