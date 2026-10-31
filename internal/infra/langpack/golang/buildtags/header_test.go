package buildtags

import "testing"

func TestHeaderLine(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		line       string
		state      commentState
		wantState  commentState
		wantHeader bool
	}{
		{"a blank line", "", outsideBlock, outsideBlock, true},
		{"a line comment", "// note", outsideBlock, outsideBlock, true},
		{"a package clause ends the header", "package main", outsideBlock, outsideBlock, false},
		{"an open block comment", "/* licence", outsideBlock, insideBlock, true},
		{"a one-line block comment", "/* licence */", outsideBlock, outsideBlock, true},
		{"a closed block then code", "/* x */ package main", outsideBlock, outsideBlock, false},
		{"inside a block, still open", "Copyright 2026 X.", insideBlock, insideBlock, true},
		{"the closing line of a block", "*/", insideBlock, outsideBlock, true},
		{"the closing line followed by a line comment", "*/ // x", insideBlock, outsideBlock, true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			gotState, gotHeader := headerLine(testCase.line, testCase.state)
			if gotState != testCase.wantState || gotHeader != testCase.wantHeader {
				t.Fatalf("headerLine(%q, %d) = %d, %v, want %d, %v",
					testCase.line, testCase.state, gotState, gotHeader,
					testCase.wantState, testCase.wantHeader)
			}
		})
	}
}

func TestBuildTagFor(t *testing.T) {
	t.Parallel()

	testCases := []headerCase{
		{
			"a go:build line after a licence block",
			"/*\nL\n*/\n\n//go:build ignore\n\npackage main\n",
			true, "//go:build ignore", true,
		},
		{
			"a go:build line inside a block comment is text",
			"/*\n//go:build ignore\n*/\n\npackage main\n",
			false, "", false,
		},
		{
			"a go:build line after the package clause is text",
			"package main\n\n//go:build ignore\n",
			false, "", false,
		},
		{
			"a +build line after a block comment is too late",
			"/* l */\n\n// +build ignore\n\npackage main\n",
			false, "", false,
		},
		{
			"a +build line not followed by a blank line is not a constraint",
			"// +build ignore\npackage main\n",
			false, "", false,
		},
		{
			"a byte order mark before go:build",
			"\xEF\xBB\xBF//go:build ignore\n\npackage main\n",
			true, "//go:build ignore", true,
		},
		{
			"an indented go:build line",
			"\t//go:build ignore\n\npackage main\n",
			true, "//go:build ignore", true,
		},
		{
			"a package clause right after a block comment",
			"/* x */ package main\n",
			false, "", false,
		},
	}

	checkHeaders(t, testCases)
}

func TestBuildTagForLegacyAndIgnore(t *testing.T) {
	t.Parallel()

	testCases := []headerCase{
		{
			"several +build lines are all required",
			"// +build linux\n// +build ignore\n\npackage main\n",
			true, "//go:build linux && ignore", true,
		},
		{
			"a +build line after a plain comment and a blank line",
			"// note\n\n// +build integration\n\npackage main\n",
			true, "// +build integration", false,
		},
		{
			"ignore required together with another tag",
			"//go:build ignore && linux\n\npackage main\n",
			true, "//go:build ignore && linux", true,
		},
		{
			"ignore as one alternative of two",
			"//go:build ignore || windows\n\npackage main\n",
			true, "//go:build ignore || windows", false,
		},
	}

	checkHeaders(t, testCases)
}

type headerCase struct {
	name           string
	source         string
	wantFound      bool
	wantConstraint string
	wantIgnore     bool
}

func checkHeaders(t *testing.T, testCases []headerCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			tag, found, err := buildTagFor("x.go", []byte(testCase.source))
			if err != nil {
				t.Fatalf("buildTagFor() error = %v", err)
			}

			if found != testCase.wantFound || tag.Constraint != testCase.wantConstraint ||
				tag.IsIgnore != testCase.wantIgnore {
				t.Fatalf("buildTagFor() = %q, ignore %v, found %v, want %q, %v, %v",
					tag.Constraint, tag.IsIgnore, found,
					testCase.wantConstraint, testCase.wantIgnore, testCase.wantFound)
			}
		})
	}
}
