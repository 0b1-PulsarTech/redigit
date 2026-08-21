package entities

import "strings"

// CommitShape says whether a commit was cut along a feature or along a layer.
// Both are legitimate and the planner records which it chose, because a history
// that mixes them silently is one nobody can audit.
type CommitShape string

const (
	// ShapeFeature is a commit whose declarations belong to one feature and
	// cross whatever directories that feature spans. The dependency is
	// satisfied inside the commit, which is what makes vertical compatible
	// with nothing being born before what needs it.
	ShapeFeature CommitShape = "feature"
	// ShapeLayer is a commit of declarations no single feature owns: the
	// bootstrap, a generator's whole output, the shared kernel. Splitting one
	// generator run across forty feature commits is less truthful, not more.
	ShapeLayer CommitShape = "layer"
)

// PlannedCommit is one commit before it is written. Reason records why the
// planner cut here, so a reviewer can disagree with the cut and not just with
// the result.
type PlannedCommit struct {
	Position Position
	Subject  string
	Shape    CommitShape
	Feature  string
	Decls    []DeclKey
	Reason   string
	IsWIP    bool
}

// DraftMarker prefixes every subject a planner invents. A generated subject is
// evidence for a reviewer, not a commit message: the previous rebuild ended up
// with 561 placeholder subjects that had to be authored afterwards, and the
// ones that slipped through described work that had moved elsewhere. The marker
// lives here, on the plan both stages meet through, rather than being spelled
// out again in each of them.
const DraftMarker = "DRAFT: "

// IsDraft reports whether a subject still needs a human or an agent to write it.
func IsDraft(subject string) bool { return strings.HasPrefix(subject, DraftMarker) }

// Plan is the reviewable artifact. No history is ever written straight from
// automatic clustering: the plan is emitted, a human or the operating agent
// adjusts it, and only then does the writer run.
type Plan struct {
	Commits []PlannedCommit
	// Unplaced names declarations no commit carries. It is never empty
	// silently: a declaration the planner could not place is a hole in the
	// final tree, and the caller must see it before writing anything.
	Unplaced []DeclKey
}

func (p Plan) Len() int { return len(p.Commits) }

// Files reports every path the plan touches, which is what the shape measures
// count over.
func (p Plan) Files() []string {
	seen := map[string]bool{}

	var out []string

	for _, commit := range p.Commits {
		for _, key := range commit.Decls {
			if !seen[key.File] {
				seen[key.File] = true

				out = append(out, key.File)
			}
		}
	}

	return out
}
