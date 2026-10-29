package gencode

import (
	"regexp"
	"strings"
)

var mockTypePattern = regexp.MustCompile(`(?m)^type Mock(\w+) struct`)

func mockedNamesIn(source []byte) []string {
	matches := mockTypePattern.FindAllSubmatch(source, -1)

	out := make([]string, 0, len(matches))

	for _, match := range matches {
		name := string(match[1])
		if strings.HasSuffix(name, "MockRecorder") {
			continue
		}

		out = append(out, name)
	}

	return out
}
