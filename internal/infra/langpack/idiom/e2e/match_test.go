package e2e

import "testing"

func TestURLOf(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		literal    string
		wantMethod string
		wantPath   string
		wantFound  bool
	}{
		{name: "a bare path", literal: "/api/x", wantPath: "/api/x", wantFound: true},
		{
			name:       "a method and a path",
			literal:    "POST /api/x",
			wantMethod: "POST",
			wantPath:   "/api/x",
			wantFound:  true,
		},
		{name: "a query is cut", literal: "/api/x?a=1", wantPath: "/api/x", wantFound: true},
		{name: "a fragment is cut", literal: "/api/x#top", wantPath: "/api/x", wantFound: true},
		{name: "a lower case word is no method", literal: "get /api/x"},
		{name: "prose", literal: "hello"},
		{name: "empty", literal: ""},
		{name: "a comment with a newline", literal: "// build tag\n\npackage x"},
		{name: "a slash text with a blank", literal: "/ some words"},
		{name: "the root alone", literal: "/", wantPath: "/", wantFound: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			method, path, found := urlOf(testCase.literal)
			if method != testCase.wantMethod || path != testCase.wantPath ||
				found != testCase.wantFound {
				t.Fatalf("urlOf(%q) = %q %q %v, want %q %q %v", testCase.literal,
					method, path, found, testCase.wantMethod, testCase.wantPath, testCase.wantFound)
			}
		})
	}
}

func TestPathMatches(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		pattern string
		url     string
		want    bool
	}{
		{name: "equal", pattern: "/users", url: "/users", want: true},
		{name: "different", pattern: "/users", url: "/orders"},
		{name: "a brace parameter", pattern: "/users/{id}", url: "/users/7", want: true},
		{name: "a colon parameter", pattern: "/users/:id", url: "/users/7", want: true},
		{name: "a parameter needs a value", pattern: "/users/{id}", url: "/users/"},
		{name: "a longer url", pattern: "/users", url: "/users/7"},
		{name: "a shorter url", pattern: "/users/{id}/x", url: "/users/7"},
		{name: "a trailing slash is a different route", pattern: "/users", url: "/users/"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := pathMatches(testCase.pattern, testCase.url); got != testCase.want {
				t.Fatalf("pathMatches(%q, %q) = %v, want %v",
					testCase.pattern, testCase.url, got, testCase.want)
			}
		})
	}
}

func TestAnswers(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		routeMethod string
		method      string
		want        bool
	}{
		{name: "a bare url", routeMethod: "GET", want: true},
		{name: "the same method", routeMethod: "GET", method: "GET", want: true},
		{name: "another method", routeMethod: "GET", method: "POST"},
		{name: "no method word", routeMethod: "HANDLEFUNC", method: "GET", want: true},
		{name: "a call named Any", routeMethod: "ANY", method: "DELETE", want: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := answers(testCase.routeMethod, testCase.method); got != testCase.want {
				t.Fatalf("answers(%q, %q) = %v, want %v",
					testCase.routeMethod, testCase.method, got, testCase.want)
			}
		})
	}
}

func TestIsLiteral(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		pattern string
		want    bool
	}{
		{pattern: "/items/me", want: true},
		{pattern: "/items/{id}"},
		{pattern: "/items/:id"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.pattern, func(t *testing.T) {
			t.Parallel()

			if got := isLiteral(testCase.pattern); got != testCase.want {
				t.Fatalf("isLiteral(%q) = %v, want %v", testCase.pattern, got, testCase.want)
			}
		})
	}
}
