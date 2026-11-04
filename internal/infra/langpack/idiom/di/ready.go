package di

import (
	"math"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

const never = entities.Position(math.MaxInt)

type (
	served struct {
		ids   []int
		isAll bool
	}

	readiness struct {
		view    catalog
		at      []entities.Position
		own     []entities.Position
		terms   [][]served
		waiters [][]int
	}
)

func newReadiness(view catalog, order entities.Ordering) *readiness {
	check := prepared(view, order)
	check.settle()

	return check
}

func prepared(view catalog, order entities.Ordering) *readiness {
	count := 0
	for _, group := range view.byType {
		count += len(group)
	}

	check := &readiness{
		view:    view,
		at:      make([]entities.Position, count),
		own:     make([]entities.Position, count),
		terms:   make([][]served, count),
		waiters: make([][]int, count),
	}

	for _, group := range view.byType {
		for _, entry := range group {
			check.enrol(entry, order)
		}
	}

	return check
}

func (check *readiness) enrol(entry registration, order entities.Ordering) {
	check.at[entry.id], check.own[entry.id] = never, never
	if at := order.Of(entry.site); at.IsSet() {
		check.own[entry.id] = at
	}

	uses := slices.Clone(entry.pulls)
	for _, need := range entry.needs {
		uses = append(uses, asUse(need))
	}

	for _, use := range uses {
		term := check.serve(use)
		check.terms[entry.id] = append(check.terms[entry.id], term)

		for _, provider := range term.ids {
			check.waiters[provider] = append(check.waiters[provider], entry.id)
		}
	}
}

func (check *readiness) serve(use resolve) served {
	serving, ambiguous := check.view.serving(use)
	if ambiguous {
		return served{isAll: use.all}
	}

	term := served{ids: make([]int, 0, len(serving)), isAll: use.all}
	for _, entry := range serving {
		term.ids = append(term.ids, entry.id)
	}

	return term
}

func (check *readiness) settle() {
	// Every registration starts as never built and only moves earlier, so a needs cycle stays never.
	queue := make([]int, 0, len(check.at))
	queued := make([]bool, len(check.at))

	for id := range check.at {
		queue = append(queue, id)
		queued[id] = true
	}

	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		queued[id] = false

		built := check.own[id]
		for _, term := range check.terms[id] {
			built = max(built, check.lowest(term))
		}

		if built >= check.at[id] {
			continue
		}

		check.at[id] = built

		for _, waiter := range check.waiters[id] {
			if !queued[waiter] {
				queue = append(queue, waiter)
				queued[waiter] = true
			}
		}
	}
}

func (check *readiness) lowest(term served) entities.Position {
	if len(term.ids) == 0 {
		return never
	}

	at := never
	if term.isAll {
		at = 0
	}

	for _, id := range term.ids {
		if term.isAll {
			at = max(at, check.at[id])
		} else {
			at = min(at, check.at[id])
		}
	}

	return at
}

func (check *readiness) readyAt(use resolve) entities.Position {
	return check.lowest(check.serve(use))
}

func lastHash(name string) int {
	for at := len(name) - 1; at >= 0; at-- {
		if name[at] == '#' {
			return at
		}
	}

	return len(name)
}
