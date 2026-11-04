package entities

import "slices"

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
	Changes  []DeclKey
	Reason   string
	IsWIP    bool
}

type Version struct {
	Text  string
	Until Position
}

// Plan is the reviewable artifact. No history is ever written straight from
// automatic clustering: the plan is emitted, a human or the operating agent
// adjusts it, and only then does the writer run.
type Plan struct {
	Commits []PlannedCommit
	// Unplaced names declarations no commit carries. It is never empty
	// silently: a declaration the planner could not place is a hole in the
	// final tree, and the caller must see it before writing anything.
	Unplaced []DeclKey
	Versions map[DeclKey][]Version
}

func (plan Plan) Len() int { return len(plan.Commits) }

func (plan Plan) PreimageAt(key DeclKey, at Position) (string, bool) {
	for _, version := range plan.Versions[key] {
		if at < version.Until {
			return version.Text, true
		}
	}

	return "", false
}

// Files reports every path the plan touches, which is what the shape measures
// count over.
func (plan Plan) Files() []string {
	seen := map[string]bool{}

	var out []string

	for _, commit := range plan.Commits {
		for _, key := range slices.Concat(commit.Decls, commit.Changes) {
			if !seen[key.File] {
				seen[key.File] = true

				out = append(out, key.File)
			}
		}
	}

	return out
}
