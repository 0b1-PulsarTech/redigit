package buildtags

import (
	"context"
	"fmt"
	"go/build/constraint"
	"runtime"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/declsource"
)

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

type BuildTag struct {
	File       string
	Constraint string
	Tags       []string
	IsIgnore   bool
}

func BuildConstraints(snap entities.Snapshot) ([]BuildTag, error) {
	var out []BuildTag

	for _, file := range snap.Files {
		source, err := declsource.ReadSource(snap, file)
		if err != nil {
			return nil, fmt.Errorf("build constraint source: %w", err)
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

func ConstraintTags(expr constraint.Expr) []string {
	seen := map[string]bool{}

	var walk func(constraint.Expr)

	walk = func(expr constraint.Expr) {
		switch typed := expr.(type) {
		case *constraint.TagExpr:
			seen[typed.Tag] = true
		case *constraint.NotExpr:
			walk(typed.X)
		case *constraint.AndExpr:
			walk(typed.X)
			walk(typed.Y)
		case *constraint.OrExpr:
			walk(typed.X)
			walk(typed.Y)
		}
	}

	walk(expr)

	tags := make([]string, 0, len(seen))
	for tag := range seen {
		tags = append(tags, tag)
	}

	return tags
}

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

		// Behind a constraint is not hidden: a //go:build linux file read on Linux would bury the
		// integration-tagged files that are really missing.
		if !expr.Eval(func(name string) bool { return known[name] }) {
			out = append(out, tag)
		}
	}

	return out, nil
}

var unixGOOS = []string{
	"aix", "android", "darwin", "dragonfly", "freebsd", "hurd", "illumos",
	"ios", "linux", "netbsd", "openbsd", "solaris",
}

func IsUnix(goos string) bool { return slices.Contains(unixGOOS, goos) }
