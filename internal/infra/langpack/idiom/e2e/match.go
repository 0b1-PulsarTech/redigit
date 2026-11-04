package e2e

import (
	"slices"
	"strings"
)

func urlOf(literal string) (method, path string, found bool) {
	if head, rest, cut := strings.Cut(literal, " /"); cut && isVerb(head) {
		method, literal = head, "/"+rest
	}

	if !strings.HasPrefix(literal, "/") {
		return "", "", false
	}

	if at := strings.IndexAny(literal, "?#"); at >= 0 {
		literal = literal[:at]
	}

	// A path holds no blank: a text that starts with a slash and has one is source or prose.
	if strings.ContainsAny(literal, " \t\r\n") {
		return "", "", false
	}

	return method, literal, true
}

func isVerb(word string) bool {
	return word != "" && strings.ToUpper(word) == word && !strings.ContainsAny(word, " /")
}

func pathMatches(pattern, url string) bool {
	want, got := strings.Split(pattern, "/"), strings.Split(url, "/")
	if len(want) != len(got) {
		return false
	}

	for idx, segment := range want {
		isParam := strings.HasPrefix(segment, "{") || strings.HasPrefix(segment, ":")
		if segment != got[idx] && (!isParam || got[idx] == "") {
			return false
		}
	}

	return true
}

func answers(routeMethod, method string) bool {
	known := []string{
		"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "CONNECT", "TRACE",
	}

	// A route call that names no method word, such as HandleFunc, answers any method.
	return method == "" || routeMethod == method || !slices.Contains(known, routeMethod)
}

func isLiteral(pattern string) bool {
	return !strings.ContainsAny(pattern, "{:")
}
