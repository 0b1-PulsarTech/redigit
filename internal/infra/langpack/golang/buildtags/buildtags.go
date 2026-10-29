package buildtags

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"go/build/constraint"
	"runtime"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/declsource"
)

// Rule registers build-constraint visibility as one of the seven Go
// rules, so it is discovered and tested alongside the others. It contributes
// no Edge or Anchor - a build tag does not change birth order - so the
// reporting itself lives in the package-level BuildConstraints function,
// which a caller with a reason to ask (the auditing use case) can call
// directly: RulePack has no return shape for "here is what an untagged index
// cannot see", only for edges and anchors.
type Rule struct{}

func (Rule) Name() string { return "build-tags" }

func (Rule) Edges(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Edge, error) {
	return nil, nil
}

func (Rule) Anchors(
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
		source, err := declsource.ReadSource(snap, file)
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

		return BuildTag{File: file, Constraint: line, Tags: ConstraintTags(expr)}, true, nil
	}

	return BuildTag{}, false, nil
}

func ConstraintTags(expr constraint.Expr) []string {
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

// Hidden reports which of tags name a constraint that satisfied does NOT
// satisfy: the set an index built with those tags never sees. Being behind a
// constraint and being invisible differ, and a "//go:build linux" file read
// normally on Linux would bury the integration-tagged files really missing.
func Hidden(tags []BuildTag, satisfied []string) ([]BuildTag, error) {
	// simplification: satisfied plus GOOS, GOARCH and "unix" is the whole
	// truth here, so a "go1.27" or "cgo" guard evaluates false and is named
	// hidden. Naming a file that was in fact read beats restoring the silence.
	// Upgrade path: evaluate against the build context gopls was started with.
	const implicitTags = 3 // GOOS, GOARCH, unix

	known := make(map[string]bool, len(satisfied)+implicitTags)
	for _, tag := range satisfied {
		known[tag] = true
	}

	known[runtime.GOOS] = true
	known[runtime.GOARCH] = true

	if IsUnix(runtime.GOOS) {
		known["unix"] = true
	}

	var out []BuildTag

	for _, tag := range tags {
		expr, err := constraint.Parse(tag.Constraint)
		if err != nil {
			return nil, fmt.Errorf("parse constraint %q in %s: %w", tag.Constraint, tag.File, err)
		}

		if !expr.Eval(func(name string) bool { return known[name] }) {
			out = append(out, tag)
		}
	}

	return out, nil
}

// unixGOOS lists the platforms for which the "unix" constraint is true, as
// go/build defines it.
var unixGOOS = []string{
	"aix", "android", "darwin", "dragonfly", "freebsd", "hurd", "illumos",
	"ios", "linux", "netbsd", "openbsd", "solaris",
}

func IsUnix(goos string) bool { return slices.Contains(unixGOOS, goos) }
