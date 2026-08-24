package golang

import (
	"regexp"
	"strings"
)

// mockTypePattern matches the type mockgen declares per interface it mocks.
// The recorder that always accompanies it is excluded by name: it mirrors
// nothing of its own.
var mockTypePattern = regexp.MustCompile(`(?m)^type Mock(\w+) struct`)

// mockedNamesIn reports the interfaces a generated mock file mirrors, read
// from its own text.
//
// From the text and not from declaration names: a generated file is ONE
// declaration, so the Mock types it holds are no longer declarations of their
// own. The naming is mockgen's convention, which is why it can be read at all.
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
