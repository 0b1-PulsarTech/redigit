package emitting

import "github.com/wrapped-owls/redigit/internal/domain/entities"

// Chunk is one declaration born in a commit, positioned in the FINAL file it
// belongs to. Span and Unit come straight from the graph, so the agent can
// see the exact body without asking the emitter to hand back the text.
type Chunk struct {
	Key  entities.DeclKey
	Span entities.Span
	Unit entities.UnitID
}

// Blob is one file's FULL content after a commit, never a diff, plus the mode
// it carries and the handle the sink returned for it.
type Blob struct {
	Path   string
	Mode   FileMode
	Handle Handle
}

// Risk is one reference this stage knows, without compiling anything, points
// at a declaration not yet born: Chunk needs Target, and Target is not born
// until BornAt. A commit with no risks probably compiles; one with risks is a
// candidate for [wip] or a deferral, and which of those is the agent's call,
// not this package's.
type Risk struct {
	Chunk  entities.DeclKey
	Target entities.DeclKey
	BornAt entities.Position
}

// Commit is one commit fully materialised: the plan's own fields carried
// forward untouched, plus the three things this stage computes from them.
type Commit struct {
	Position entities.Position
	Subject  string
	Shape    entities.CommitShape
	Feature  string
	Reason   string
	IsWIP    bool

	// Chunks are in topological order among themselves (see topoSort): a
	// prefix of them never breaks a dependency this same commit already
	// satisfied. It does NOT guarantee the prefix compiles - a declaration
	// may depend on something born in no commit at all yet, and the target
	// history itself accepts a commit that does not compile, marked [wip].
	Chunks []Chunk
	Blobs  []Blob
	Risks  []Risk
}

// IsEmpty reports a commit that changes nothing. An empty commit wearing a
// feature subject claims work that happened somewhere else.
func (c Commit) IsEmpty() bool { return len(c.Blobs) == 0 }
