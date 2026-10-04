package golang

import (
	"go/build/constraint"
	"maps"
	"path"
	"runtime"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// hiddenPasses is one extra load per tag set that shows the Go files the run's
// tags hide. A file behind "//go:build !dev" is invisible to a run under "dev",
// so nothing in it is ever type-checked and what it reads lands after it: the
// trees between fail with "undefined: NewGatewayVerifier". Each pass covers only
// the packages holding a hidden file, under the tags that satisfy it.
func hiddenPasses(snap entities.Snapshot, tags []string, modules []module) ([]loadPass, error) {
	constrained, err := BuildConstraints(snap)
	if err != nil {
		return nil, err
	}

	hidden, err := Hidden(constrained, tags)
	if err != nil {
		return nil, err
	}

	byTags := map[string]*loadPass{}

	for _, file := range hidden {
		pkg, inModule := packageOfFile(modules, file.File)
		if !inModule || !strings.HasSuffix(file.File, ".go") {
			continue
		}

		showing, satisfiable := showingTags(file.Constraint, tags)
		if !satisfiable {
			continue
		}

		key := strings.Join(showing, ",")
		if byTags[key] == nil {
			byTags[key] = &loadPass{tags: showing}
		}

		if !slices.Contains(byTags[key].patterns, pkg) {
			byTags[key].patterns = append(byTags[key].patterns, pkg)
		}
	}

	passes := make([]loadPass, 0, len(byTags))

	for _, key := range slices.Sorted(maps.Keys(byTags)) {
		slices.Sort(byTags[key].patterns)

		passes = append(passes, *byTags[key])
	}

	return passes, nil
}

// packageOfFile is the import path of the package a file belongs to: its
// directory under the nearest module that holds it.
func packageOfFile(modules []module, file string) (string, bool) {
	best := -1

	for index, found := range modules {
		inside := found.dir == "." || strings.HasPrefix(file, found.dir+"/")
		if inside && (best < 0 || len(found.dir) > len(modules[best].dir)) {
			best = index
		}
	}

	if best < 0 {
		return "", false
	}

	rel := strings.TrimPrefix(path.Dir(file), modules[best].dir)

	return modules[best].path + strings.TrimSuffix(rel, "/"), true
}

// showingTags is the run's tags changed just enough to satisfy a constraint:
// every tag the constraint names is tried on and off, so "!dev" is satisfied by
// dropping dev and "integration && dev" by keeping both. It reports false for a
// constraint nothing satisfies.
func showingTags(line string, tags []string) ([]string, bool) {
	expr, err := constraint.Parse(line)
	if err != nil {
		return nil, false
	}

	named := constraintTags(expr)
	slices.Sort(named)

	// Tags a constraint can name by hand are few; anything wider is not a build
	// switch a person wrote.
	const maxNamed = 8
	if len(named) > maxNamed {
		return nil, false
	}

	for mask := range 1 << len(named) {
		chosen := slices.DeleteFunc(slices.Clone(tags), func(tag string) bool {
			return slices.Contains(named, tag)
		})

		for bit, name := range named {
			if mask&(1<<bit) != 0 {
				chosen = append(chosen, name)
			}
		}

		if satisfies(expr, chosen) {
			slices.Sort(chosen)

			return slices.Compact(chosen), true
		}
	}

	return nil, false
}

func satisfies(expr constraint.Expr, tags []string) bool {
	return expr.Eval(func(name string) bool {
		return slices.Contains(tags, name) || name == runtime.GOOS || name == runtime.GOARCH ||
			(name == "unix" && isUnix(runtime.GOOS))
	})
}
