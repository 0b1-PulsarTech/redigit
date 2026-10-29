package planning

import (
	"cmp"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/birth"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

type FeatureID = reach.FeatureID

// Capabilities is a caller-supplied answer to which slice each declaration
// belongs to, replacing the reachability walk. Reachability asks "what does
// this handler need", which on a real repository left 28% of declarations
// reached by no entry point and 14% reached by several: 42% in layer commits
// with no slice of their own.
type Capabilities map[entities.DeclKey]FeatureID

// ownershipFrom builds the ownership a caller supplied. Every declaration the
// map names belongs to exactly one slice, so nothing is shared and nothing is
// unreached; a declaration the map does not name stays SharedFeature, which is
// where the unreached already went.
func ownershipFrom(capabilities Capabilities, graph entities.Graph) ownership {
	own := ownership{
		owner: make(map[entities.DeclKey]FeatureID, graph.Len()),
		reach: make(map[entities.DeclKey]int, graph.Len()),
	}

	seen := map[FeatureID]bool{}

	for _, decl := range graph.Decls() {
		feature, named := capabilities[decl.Key]
		if !named {
			feature = reach.SharedFeature
		}

		own.owner[decl.Key] = feature
		own.reach[decl.Key] = 1

		if feature != reach.SharedFeature && !seen[feature] {
			seen[feature] = true

			own.order = append(own.order, feature)
		}
	}

	slices.SortFunc(own.order, func(a, b FeatureID) int { return cmp.Compare(a, b) })

	return own
}

// emitByCapability walks every declaration once in birth order, starting a new
// commit whenever the slice changes or MaxFiles is reached. One pass, not a
// loop over slices: a slice's members and what it pulls from elsewhere have to
// interleave by position, because a pulled declaration may itself need a
// member of the slice that pulled it.
func (p Planner) emitByCapability(
	graph entities.Graph,
	order entities.Ordering,
	own ownership,
) entities.Plan {
	atoms := make(map[entities.DeclKey]string, graph.Len())
	keys := make([]entities.DeclKey, 0, graph.Len())

	for _, decl := range graph.Decls() {
		keys = append(keys, decl.Key)

		if decl.Atom != "" {
			atoms[decl.Key] = decl.Atom
		}
	}

	// The affinity a tie is resolved by: the atom first, because its members
	// must end up in ONE commit, then the slice.
	keys = birth.SortForBirth(graph, order, keys, func(key entities.DeclKey) FeatureID {
		if atom, grouped := atoms[key]; grouped {
			return FeatureID(atom)
		}

		return own.owner[key]
	})

	run := &commitRun{
		maxFiles: p.cfg.MaxFiles,
		maxLines: p.cfg.MaxAddedLines,
		files:    map[string]bool{},
	}

	for _, key := range keys {
		run.take(key, own.owner[key], atoms[key], weightOf(graph, key))
	}

	run.flush()

	return entities.Plan{Commits: run.commits}
}

// commitRun accumulates declarations into commits as they arrive in birth
// order, closing one whenever the next declaration cannot join it.
type commitRun struct {
	maxFiles int
	maxLines int

	commits []entities.PlannedCommit
	current []entities.DeclKey
	files   map[string]bool
	slice   FeatureID
	atom    string
	pkg     string
	started bool
	size    tally
}

// take adds one declaration, closing the open commit when it belongs to
// another package, the open one spans MaxFiles, or MaxAddedLines is passed.
//
// The cut follows the PACKAGE, the slice deciding order rather than
// boundaries: cutting on the slice put 31% of commits at one file each,
// because the slice changes at almost every commit.
func (r *commitRun) take(key entities.DeclKey, owner FeatureID, atom string, size weight) {
	// A commit never splits an atom, whatever else changes. Its members were
	// written by a tool in one pass, and they carry different slices only
	// because each was DATED separately.
	inAtom := r.started && atom != "" && atom == r.atom

	tooWide := !r.files[key.File] && len(r.files) >= r.maxFiles
	tooLong := r.size.overflows(size, r.maxLines)

	if r.started && !inAtom && (dirOf(key.File) != r.pkg || tooWide || tooLong) {
		r.flush()
	}

	r.slice = owner
	r.atom = atom
	r.pkg = dirOf(key.File)
	r.started = true
	r.files[key.File] = true
	r.size.add(size)

	r.current = append(r.current, key)
}

func (r *commitRun) flush() {
	if len(r.current) == 0 {
		return
	}

	r.commits = append(r.commits, entities.PlannedCommit{
		Position: entities.Position(len(r.commits)),
		Subject:  draftSubject(entities.ShapeFeature, r.slice, r.current),
		Shape:    entities.ShapeFeature,
		Feature:  string(r.slice),
		Decls:    slices.Clone(r.current),
		Reason: annotate(
			"one package, in the order the fixed point settled", r.size, r.maxLines,
		),
	})

	r.current = r.current[:0]
	r.files = map[string]bool{}
	r.atom = ""
	r.pkg = ""
	r.size = tally{}
}

// dirOf is the package a file belongs to: its directory, which is what Go
// compiles as a unit.
func dirOf(file string) string {
	if cut := strings.LastIndexByte(file, '/'); cut >= 0 {
		return file[:cut]
	}

	return "."
}
