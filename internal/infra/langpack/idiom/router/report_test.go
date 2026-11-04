package router

import "testing"

func TestGateStatesItsLimits(t *testing.T) {
	t.Parallel()

	testCases := []gateCase{
		{
			name: "every limit is stated",
			wantText: []string{
				"a route registered through a func-typed variable or field has no Call fact",
				"a group opened by assignment or by a chained call lends its routes no prefix",
				"only the nearest enclosing group call lends its prefix",
				"the method is the name of the registering call",
				"the last func argument is the handler and the earlier ones are read as middleware",
				"a mount handed on through a variable is not seen",
				"only the first statement calling a mount, in file and source order, is compared",
				"a method of a type that receives route calls is read as the router's own",
				"HANDLEFUNC and HANDLE are read as two methods, so a repeat across them is not seen",
				"a second func init in one file is not read",
				"a func literal held by a package-level var is not read",
				"a call through an anonymous interface type is not seen",
				"a loop registering routes is one route whose path is no string constant",
				"the method of a route may read as HANDLEFUNC, ANY, ADD or METHOD",
				"a route in a nested module, one with a go.mod below the root, is not read",
			},
		},
		{
			name: "a middleware read as the handler is stated both ways",
			wantText: []string{
				"a middleware before a func literal handler, or after a named one, is read as the handler",
			},
		},
		{
			name: "placed compares positions of the ordering, not the commits of the plan",
			wantText: []string{
				"placed compares positions of the ordering, not the commits of the plan: " +
					"a mount and its caller at one position may land in different commits",
			},
		},
		{
			name: "groupMethods is stated to join equal paths across the groups of a function",
			wantText: []string{
				"groupMethods joins the statements of an equal path, prefix included, " +
					"across groups of one function, so two groups that lend no prefix " +
					"and share a relative path read as one resource",
			},
		},
	}

	requireGate(t, testCases)
}
