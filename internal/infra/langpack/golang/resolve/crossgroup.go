package resolve

import (
	"go/types"
	"slices"
	"strconv"

	"golang.org/x/tools/go/packages"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type (
	shape struct {
		faces   map[site][]string
		kinds   [][]string
		crossed map[site]bool
	}

	implemented struct {
		shape

		implementors map[site][]entities.Location
	}
)

func shapeOf(all []namedType) shape {
	out := shape{faces: map[site][]string{}, crossed: map[site]bool{}}

	for _, entry := range all {
		if entry.target != nil && entry.target.NumMethods() > 0 {
			keys := make([]string, 0, entry.target.NumMethods())
			for want := range entry.target.Methods() {
				keys = append(keys, keyOf(want))
			}

			slices.Sort(keys)

			out.faces[entry.at] = keys
		}

		if !entry.alias && !entry.local {
			out.kinds = append(out.kinds, methodKeys(entry.named))
		}
	}

	return out
}

func methodKeys(named *types.Named) []string {
	receiver := types.Type(types.NewPointer(named))
	if types.IsInterface(named) {
		receiver = named
	}

	set := types.NewMethodSet(receiver)
	keys := make([]string, 0, set.Len())

	for method := range set.Methods() {
		if have, isFunc := method.Obj().(*types.Func); isFunc {
			keys = append(keys, keyOf(have))
		}
	}

	slices.Sort(keys)

	return keys
}

func keyOf(method *types.Func) string {
	sig, _ := method.Type().(*types.Signature)

	return method.Name() + "/" + strconv.Itoa(sig.Params().Len()) + "/" +
		strconv.Itoa(sig.Results().Len())
}

func settle(groups []index) {
	settleUnread(groups)
	settleCross(groups)
}

func settleCross(groups []index) {
	if len(groups) <= 1 {
		return
	}

	owners := make([]map[string][][]string, len(groups))

	for idx, group := range groups {
		owners[idx] = map[string][][]string{}

		for _, keys := range group.kinds {
			for _, key := range keys {
				owners[idx][key] = append(owners[idx][key], keys)
			}
		}
	}

	for idx, group := range groups {
		for at, keys := range group.faces {
			if metElsewhere(owners, idx, keys) {
				group.crossed[at] = true
			}
		}
	}

	for idx := range groups {
		groups[idx].faces, groups[idx].kinds = nil, nil
	}
}

func metElsewhere(owners []map[string][][]string, group int, keys []string) bool {
	for idx, bucket := range owners {
		if idx == group {
			continue
		}

		rarest := slices.MinFunc(keys, func(left, right string) int {
			return len(bucket[left]) - len(bucket[right])
		})

		if slices.ContainsFunc(bucket[rarest], func(have []string) bool {
			return covers(have, keys)
		}) {
			return true
		}
	}

	return false
}

func indexGroups(groups [][]*packages.Package, where *places) []index {
	groups = slices.DeleteFunc(slices.Clone(groups), func(group []*packages.Package) bool {
		return len(group) == 0
	})
	built := make([]index, 0, len(groups))

	for _, group := range groups {
		all := namedTypes(group, where)
		entry := index{
			usage:       collectUses(group, where),
			implemented: collectImplementors(all),
			covered:     where.covered(group),
			cgo:         unseen(group),
		}

		// A single group has nothing to meet, so its shape would only be held for nothing.
		if len(groups) > 1 {
			entry.shape = shapeOf(all)
			entry.refuseTestClauses(group, where)
		}

		built = append(built, entry)
	}

	settle(built)

	return built
}

func covers(have, want []string) bool {
	return !slices.ContainsFunc(want, func(key string) bool {
		_, found := slices.BinarySearch(have, key)

		return !found
	})
}

func (built index) refuses(where site) bool {
	// A type of another group is another type-check, so it cannot be tried; gopls loads both.
	return built.cgo || built.crossed[where]
}
