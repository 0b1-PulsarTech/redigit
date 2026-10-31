package buildtags

import (
	"bufio"
	"bytes"
	"fmt"
	"go/build/constraint"
	"strings"
)

type commentState int

const (
	outsideBlock commentState = iota
	insideBlock
)

func headerLine(line string, state commentState) (next commentState, isHeader bool) {
	for line != "" {
		switch {
		case state == insideBlock:
			_, rest, closed := strings.Cut(line, "*/")
			if !closed {
				return insideBlock, true
			}

			state, line = outsideBlock, strings.TrimSpace(rest)
		case strings.HasPrefix(line, "//"):
			return outsideBlock, true
		case strings.HasPrefix(line, "/*"):
			state, line = insideBlock, line[len("/*"):]
		default:
			return outsideBlock, false
		}
	}

	return state, true
}

func buildTagFor(file string, source []byte) (BuildTag, bool, error) {
	scanner := bufio.NewScanner(bytes.NewReader(source))
	state := outsideBlock

	var pending, accepted []string

	ended := false

	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\ufeff"))
		if state == outsideBlock && constraint.IsGoBuild(line) {
			return tagFor(file, []string{line})
		}

		// Like go/build, a +build line counts only before the first code or block line, then a blank.
		if !ended && constraint.IsPlusBuild(line) {
			pending = append(pending, line)
		}

		if line == "" && !ended {
			accepted, pending = append(accepted, pending...), nil
		}

		ended = ended || (line != "" && !strings.HasPrefix(line, "//"))

		// A block comment header hides nothing: go/build reads a go:build line past it.
		var isHeader bool
		if state, isHeader = headerLine(line, state); !isHeader {
			break
		}
	}

	if len(accepted) == 0 {
		return BuildTag{}, false, nil
	}

	return tagFor(file, accepted)
}

func tagFor(file string, lines []string) (BuildTag, bool, error) {
	var joined constraint.Expr

	for _, line := range lines {
		expr, err := constraint.Parse(line)
		if err != nil {
			return BuildTag{}, false, fmt.Errorf("parse constraint %q in %s: %w", line, file, err)
		}

		if joined == nil {
			joined = expr
		} else {
			joined = &constraint.AndExpr{X: joined, Y: expr}
		}
	}

	text := lines[0]
	if len(lines) > 1 {
		text = "//go:build " + joined.String()
	}

	tag := BuildTag{
		File: file, Constraint: text, Tags: ConstraintTags(joined), IsIgnore: isIgnore(joined),
	}

	return tag, true, nil
}

func isIgnore(expr constraint.Expr) bool {
	switch typed := expr.(type) {
	case *constraint.TagExpr:
		return typed.Tag == "ignore"
	case *constraint.AndExpr:
		return isIgnore(typed.X) || isIgnore(typed.Y)
	}

	return false
}
