package idiom

import "github.com/wrapped-owls/redigit/internal/domain/entities"

type cycleGuard struct {
	slots   map[entities.DeclKey]int
	plain   [][]int
	accept  map[int][]int
	comp    []int
	visited []int
	loop    []int
	stamp   int
	stack   []int
}

func newGuard(edges []entities.Edge) *cycleGuard {
	if !hasDI(edges) {
		return nil
	}

	guard := &cycleGuard{slots: map[entities.DeclKey]int{}, accept: map[int][]int{}}

	var every [][]int

	slot := func(key entities.DeclKey) int {
		at, known := guard.slots[key]
		if !known {
			at = len(every)
			guard.slots[key] = at
			every, guard.plain = append(every, nil), append(guard.plain, nil)
		}

		return at
	}

	for _, edge := range edges {
		if isCoBirth(edge) {
			continue
		}

		from, to := slot(edge.Dependent), slot(edge.Provider)

		every[from] = append(every[from], to)

		if !isDI(edge.Reason) {
			guard.plain[from] = append(guard.plain[from], to)
		}
	}

	guard.comp, guard.visited = components(every), make([]int, len(every))
	guard.loop = components(guard.plain)

	return guard
}

func isCoBirth(edge entities.Edge) bool {
	return edge.Reason == entities.ReasonHosts && edge.Provider.Kind.IsCode()
}

func hasDI(edges []entities.Edge) bool {
	for _, edge := range edges {
		if isDI(edge.Reason) {
			return true
		}
	}

	return false
}

func (guard *cycleGuard) accepts(edge entities.Edge) bool {
	from, to := guard.slots[edge.Dependent], guard.slots[edge.Provider]

	// An edge between two components lies on no cycle, whatever else is dropped.
	if from != to && guard.comp[from] != guard.comp[to] {
		return true
	}

	// Two ends of a cycle of non-DI edges reach each other without a search.
	if from == to || guard.loop[from] == guard.loop[to] || guard.reaches(to, from) {
		return false
	}

	guard.accept[from] = append(guard.accept[from], to)

	return true
}

func (guard *cycleGuard) reaches(start, goal int) bool {
	guard.stamp++
	guard.stack = guard.stack[:0]
	guard.push(start)

	for len(guard.stack) > 0 {
		at := guard.stack[len(guard.stack)-1]
		guard.stack = guard.stack[:len(guard.stack)-1]

		if at == goal {
			return true
		}

		for _, next := range guard.accept[at] {
			guard.push(next)
		}

		// Needs other than DI are every reason the orderer counts for a cycle: calls and function
		// values, var and field references, implements, method sets, encloses and bodies. A hosts
		// edge onto code is co-birth, not a need, so it is left out as ordering/hostsplit.go does.
		for _, next := range guard.plain[at] {
			if guard.comp[next] == guard.comp[goal] {
				guard.push(next)
			}
		}
	}

	return false
}

func (guard *cycleGuard) push(at int) {
	if guard.visited[at] != guard.stamp {
		guard.visited[at] = guard.stamp
		guard.stack = append(guard.stack, at)
	}
}

func components(adjacent [][]int) []int {
	var (
		order, low = make([]int, len(adjacent)), make([]int, len(adjacent))
		comp       = make([]int, len(adjacent))
		onStack    = make([]bool, len(adjacent))
		stack      []int
		counter    int
		visit      func(at int)
	)

	visit = func(at int) {
		counter++
		order[at], low[at] = counter, counter

		stack, onStack[at] = append(stack, at), true

		for _, next := range adjacent[at] {
			if order[next] == 0 {
				visit(next)
			}

			if onStack[next] {
				low[at] = min(low[at], low[next])
			}
		}

		if low[at] != order[at] {
			return
		}

		for top := -1; top != at; {
			top = stack[len(stack)-1]
			stack, onStack[top], comp[top] = stack[:len(stack)-1], false, order[at]
		}
	}

	for at := range adjacent {
		if order[at] == 0 {
			visit(at)
		}
	}

	return comp
}
