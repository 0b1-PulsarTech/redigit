package router

import (
	"fmt"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func (count tally) excluded() []string {
	var lines []string

	if count.matched == 0 {
		lines = append(
			lines,
			"handle matched no call site: a pattern matches the short name, "+
				"such as Router.GET, or the qualified one, such as github.com/go-fuego/fuego.Get, "+
				"and * does not cross /, so *.Get misses that func",
		)
	}

	lines = append(lines, []string{
		fmt.Sprintf(
			"%d of %d mounts are placed apart from their first caller",
			count.apart,
			count.mounts,
		),
		fmt.Sprintf(
			"%d mounts are called only from a func that was not cut into statements: "+
				"not compared with a caller",
			count.unmet,
		),
		fmt.Sprintf(
			"%d of %d routes repeat the method and path of an earlier route in the same function",
			count.repeats, count.routes,
		),
		fmt.Sprintf(
			"%d routes repeat the method and path of an earlier route of another function: not judged",
			count.crossed,
		),
		fmt.Sprintf(
			"%d of %d routes are placed before their handler",
			count.early,
			count.routes,
		),
		fmt.Sprintf(
			"%d routes whose path is no string constant: not in the total", count.blind,
		),
		fmt.Sprintf(
			"%d routes whose constants are no path (relative, empty, a host): not in the total",
			count.nonPath,
		),
		fmt.Sprintf(
			"%d routes with no handler the facts name (a func literal, a wrapped handler "+
				"or a func value): not in the total",
			count.inline,
		),
		fmt.Sprintf(
			"%d mounts and %d routes the ordering never placed",
			count.unplacedMounts,
			count.unplacedRoutes,
		),
	}...)

	return append(lines, limits()...)
}

func notMeasured(err error) entities.Measure {
	return entities.Measure{
		Name:     measureName,
		Excluded: []string{fmt.Sprintf("not measured: %v", err)},
	}
}

func limits() []string {
	return []string{
		"a route registered through a func-typed variable or field has no Call fact, so it is not counted",
		"a group opened by assignment or by a chained call lends its routes no prefix, " +
			"so equal relative paths of two groups read as repeats",
		"only the nearest enclosing group call lends its prefix; the prefix of a group around it is not applied",
		"the method is the name of the registering call or the verb of a METHOD /path pattern, " +
			"so a verb passed as another argument reads as the call's name",
		"the last func argument is the handler and the earlier ones are read as middleware",
		"a mount handed on through a variable is not seen",
		"only the first statement calling a mount, in file and source order, is compared with it",
		"a method of a type that receives route calls is read as the router's own, never as a mount",
		"HANDLEFUNC and HANDLE are read as two methods, so a repeat across them is not seen",
		"a second func init in one file is not read, so the routes it registers are not counted",
		"a func literal held by a package-level var is not read",
		"a call through an anonymous interface type is not seen",
		"a loop registering routes is one route whose path is no string constant",
		"groupMethods joins the statements of an equal path, prefix included, " +
			"across groups of one function, so two groups that lend no prefix " +
			"and share a relative path read as one resource",
		"a middleware before a func literal handler, or after a named one, is read as the handler",
		"the method of a route may read as HANDLEFUNC, ANY, ADD or METHOD, the name of the call",
		"placed compares positions of the ordering, not the commits of the plan: " +
			"a mount and its caller at one position may land in different commits",
		"a route in a nested module, one with a go.mod below the root, is not read",
	}
}
