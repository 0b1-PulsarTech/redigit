package golang

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"go/build/constraint"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// buildTagsRule registers build-constraint visibility as one of the seven Go
// rules, so it is discovered and tested alongside the others. It contributes
// no Edge or Anchor - a build tag does not change birth order - so the
// reporting itself lives in the package-level BuildConstraints function,
// which a caller with a reason to ask (the auditing use case) can call
// directly: RulePack has no return shape for "here is what an untagged index
// cannot see", only for edges and anchors.
type buildTagsRule struct{}

func (buildTagsRule) Name() string { return "build-tags" }

func (buildTagsRule) Edges(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Edge, error) {
	return nil, nil
}

func (buildTagsRule) Anchors(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}

// BuildTag records that a file is gated behind a build constraint, and the
// tags that expression names. This is the 194-file blind spot named in the
// design doc: an index built without these tags never sees the file at all,
// and a caller has no way to know that unless this says so.
type BuildTag struct {
	File       string
	Constraint string
	Tags       []string
}

// BuildConstraints reports every file behind a build constraint (//go:build,
// or the legacy // +build), in Files order.
func BuildConstraints(snap entities.Snapshot) ([]BuildTag, error) {
	var out []BuildTag

	for _, file := range snap.Files {
		source, err := readSource(snap, file)
		if err != nil {
			return nil, err
		}

		tag, found, err := buildTagFor(file, source)
		if err != nil {
			return nil, err
		}

		if found {
			out = append(out, tag)
		}
	}

	return out, nil
}

func buildTagFor(file string, source []byte) (BuildTag, bool, error) {
	scanner := bufio.NewScanner(bytes.NewReader(source))

	for scanner.Scan() {
		line := scanner.Text()
		if !constraint.IsGoBuild(line) && !constraint.IsPlusBuild(line) {
			if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}

			break
		}

		expr, err := constraint.Parse(line)
		if err != nil {
			return BuildTag{}, false, fmt.Errorf("parse constraint %q in %s: %w", line, file, err)
		}

		return BuildTag{File: file, Constraint: line, Tags: constraintTags(expr)}, true, nil
	}

	return BuildTag{}, false, nil
}

func constraintTags(expr constraint.Expr) []string {
	seen := map[string]bool{}

	var walk func(constraint.Expr)

	walk = func(e constraint.Expr) {
		switch n := e.(type) {
		case *constraint.TagExpr:
			seen[n.Tag] = true
		case *constraint.NotExpr:
			walk(n.X)
		case *constraint.AndExpr:
			walk(n.X)
			walk(n.Y)
		case *constraint.OrExpr:
			walk(n.X)
			walk(n.Y)
		}
	}

	walk(expr)

	tags := make([]string, 0, len(seen))
	for tag := range seen {
		tags = append(tags, tag)
	}

	return tags
}
