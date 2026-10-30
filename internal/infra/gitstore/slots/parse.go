package slots

import (
	"bytes"
	"errors"
	"path"
	"slices"
	"strconv"
	"strings"
)

const (
	headerFields = 3
	goWorkName   = "go.work"
	goLanguage   = "go"
)

var (
	ErrBadTree = errors.New("malformed git tree listing")
	ErrBadBlob = errors.New("malformed git blob stream")
)

type (
	markerFile struct {
		dir      string
		language string
		name     string
		blob     string
	}

	treeListing struct {
		files []markerFile
		works []markerFile
		trees map[string]string
		goSrc []string

		hasLink bool
	}
)

func parseTree(out []byte, markers map[string]string) (treeListing, error) {
	found := treeListing{trees: map[string]string{}}

	for entry := range bytes.SplitSeq(bytes.TrimSuffix(out, []byte{0}), []byte{0}) {
		if len(entry) == 0 {
			continue
		}

		header, name, hasTab := bytes.Cut(entry, []byte{'\t'})
		fields := strings.Fields(string(header))

		if !hasTab || len(fields) != headerFields {
			return treeListing{}, ErrBadTree
		}

		switch fields[1] {
		case "tree":
			found.trees[string(name)] = fields[2]
		case "blob":
			// A link can pull bytes from anywhere in the tree, so no subtree key vouches for it.
			if fields[0] == "120000" && !inTestdata(path.Dir(string(name))) {
				found.hasLink = true
			}

			found.addBlob(string(name), fields[2], markers)
		case "commit":
			// A submodule vendor holds bytes the tree does not list, so only the commit vouches.
			if path.Base(string(name)) == "vendor" {
				found.hasLink = true
			}
		}
	}

	return found, nil
}

func (found *treeListing) addBlob(name, blob string, markers map[string]string) {
	dir, base := path.Dir(name), path.Base(name)
	if isToolPath(dir) {
		return
	}

	if strings.HasSuffix(base, ".go") && !strings.HasPrefix(base, "_") &&
		!strings.HasPrefix(base, ".") {
		found.goSrc = append(found.goSrc, name)
	}

	if language, isMarker := markers[base]; isMarker {
		found.files = append(found.files, markerFile{dir: dir, language: language, blob: blob})
	}

	if base == goWorkName || base == goWorkName+".sum" {
		found.works = append(found.works, markerFile{dir: dir, name: base, blob: blob})
	}
}

func inTestdata(dir string) bool {
	return slices.Contains(strings.Split(dir, "/"), "testdata")
}

func isToolPath(dir string) bool {
	if dir == "." {
		return false
	}

	for element := range strings.SplitSeq(dir, "/") {
		if element == "testdata" || element == "vendor" ||
			strings.HasPrefix(element, "_") || strings.HasPrefix(element, ".") {
			return true
		}
	}

	return false
}

func parseBlobs(out []byte, oids []string) ([][]byte, error) {
	contents := make([][]byte, 0, len(oids))
	rest := out

	for range oids {
		line, after, hasLine := bytes.Cut(rest, []byte{'\n'})
		fields := strings.Fields(string(line))

		if !hasLine || len(fields) != headerFields || fields[1] != "blob" {
			return nil, ErrBadBlob
		}

		size, err := strconv.Atoi(fields[2])
		if err != nil || size < 0 || len(after) < size+1 {
			return nil, ErrBadBlob
		}

		contents = append(contents, after[:size])
		rest = after[size+1:]
	}

	return contents, nil
}
