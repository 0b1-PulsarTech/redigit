package typed

import (
	"context"
	"fmt"
	"go/build/constraint"
	"maps"
	"path"
	"runtime"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/buildtags"
)

func hiddenPasses(snap entities.Snapshot, tags []string, modules []module) ([]loadPass, error) {
	constrained, err := buildtags.BuildConstraints(snap)
	if err != nil {
		return nil, fmt.Errorf("build constraints: %w", err)
	}

	hidden, err := buildtags.Hidden(constrained, tags)
	if err != nil {
		return nil, fmt.Errorf("hidden passes: %w", err)
	}

	// A file hidden by the run tags is never type-checked, so what it reads lands after it; one extra
	// load per tag set shows it, limited to packages holding a hidden file.
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

func ownerOf(modules []module, file string) (module, bool) {
	best := -1

	for index, found := range modules {
		inside := found.dir == "." || strings.HasPrefix(file, found.dir+"/")
		if inside && (best < 0 || len(found.dir) > len(modules[best].dir)) {
			best = index
		}
	}

	if best < 0 {
		return module{}, false
	}

	return modules[best], true
}

func packageOfFile(modules []module, file string) (string, bool) {
	owner, inside := ownerOf(modules, file)
	if !inside {
		return "", false
	}

	rel := strings.TrimPrefix(path.Dir(file), owner.dir)

	return owner.path + strings.TrimSuffix(rel, "/"), true
}

func showingTags(line string, tags []string) ([]string, bool) {
	expr, err := constraint.Parse(line)
	if err != nil {
		return nil, false
	}

	named := buildtags.ConstraintTags(expr)
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
			(name == "unix" && buildtags.IsUnix(runtime.GOOS))
	})
}

func (source *Source) Packages(
	ctx context.Context,
	snap entities.Snapshot,
) ([]*packages.Package, error) {
	loaded, err := source.load(ctx, snap)
	if err != nil || len(loaded) == 0 {
		return nil, err
	}

	return loaded[0], nil
}

func (source *Source) load(
	ctx context.Context,
	snap entities.Snapshot,
) ([][]*packages.Package, error) {
	// Loading once is sound only because one run indexes one snapshot.
	loaded, err := source.loaded.Do(
		ctx,
		func(loadCtx context.Context) ([][]*packages.Package, error) {
			return source.check(loadCtx, snap)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("typed load: %w", err)
	}

	return loaded, nil
}
