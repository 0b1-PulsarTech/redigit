package planning

import (
	"fmt"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// weight is what one declaration adds to a commit: its lines, and the frame
// of its file when the commit does not hold the file yet.
type weight struct {
	file      string
	lines     int
	frame     int
	generated bool
}

func weightOf(graph entities.Graph, key entities.DeclKey) weight {
	decl, found := graph.Decl(key)
	if !found {
		return weight{}
	}

	return weight{
		file:      key.File,
		lines:     decl.Lines,
		frame:     decl.Frame,
		generated: decl.Generated,
	}
}

// tally is what the open commit holds so far.
//
// A published history carried commits of 2000 to 4361 hand-written lines, more
// than a person reads. Generated lines may be any size, but past the cap they
// bury the hand-written lines beside them, so they take a commit of their own.
type tally struct {
	hand      int
	generated int
	files     map[string]bool
}

// cost is what w adds: a file's frame is added by the first declaration of it.
func (t tally) cost(w weight) int {
	if t.files[w.file] {
		return w.lines
	}

	return w.lines + w.frame
}

func (t *tally) add(w weight) {
	if t.files == nil {
		t.files = map[string]bool{}
	}

	cost := t.cost(w)
	t.files[w.file] = true

	if w.generated {
		t.generated += cost

		return
	}

	t.hand += cost
}

func tallyOf(graph entities.Graph, keys []entities.DeclKey) tally {
	var out tally

	for _, key := range keys {
		out.add(weightOf(graph, key))
	}

	return out
}

// overflows reports whether w must open a new commit. An empty commit never
// overflows: a declaration over the cap by itself has nowhere else to go.
func (t tally) overflows(w weight, limit int) bool {
	if t.hand == 0 && t.generated == 0 {
		return false
	}

	cost := t.cost(w)
	if w.generated {
		return t.hand > 0 && t.generated+cost > limit
	}

	return t.generated > limit || (t.hand > 0 && t.hand+cost > limit)
}

// annotate names a commit left over the cap, because a cut that cannot be made
// is a fact the reviewer needs, not one to leave for the line count to reveal.
func annotate(reason string, t tally, limit int) string {
	if t.hand <= limit {
		return reason
	}

	return fmt.Sprintf(
		"%s; %d hand-written lines, over the %d cap: an atom or one declaration cannot be split",
		reason, t.hand, limit,
	)
}
