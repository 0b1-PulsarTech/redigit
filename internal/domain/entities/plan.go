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

// DraftMarker prefixes every subject a planner invents: a generated subject is
// evidence for a reviewer, not a commit message. The previous rebuild ended up
// with 561 placeholders, and the ones that slipped through described work that
// had moved elsewhere.
const DraftMarker = "DRAFT: "

// IsDraft reports whether a subject still needs a human or an agent to write it.
func IsDraft(subject string) bool { return strings.HasPrefix(subject, DraftMarker) }

// WIPMarker declares a commit leaving a piece incomplete: a type without all
// its methods, a declaration reaching for something unborn. Only the final tree
// must compile, and unmarked, an honest vertical slice is indistinguishable
// from a defect - which reported 311 incompletenesses as 311 failures.
const WIPMarker = "[wip] "

// IsWIPSubject reports whether a subject already carries the marker.
func IsWIPSubject(subject string) bool { return strings.Contains(subject, WIPMarker) }

// MarkWIP puts the marker where the conventional-commit format wants it:
// after "type(scope): " and before the description, keeping any draft prefix
// ahead of both. Marking twice changes nothing.
func MarkWIP(subject string) string {
	if IsWIPSubject(subject) {
		return subject
	}

	prefix := ""

	body := subject

	if IsDraft(subject) {
		prefix = DraftMarker
		body = strings.TrimPrefix(subject, DraftMarker)
	}

	cut := strings.Index(body, ": ")
	if cut < 0 {
		return prefix + WIPMarker + body
	}

	return prefix + body[:cut+2] + WIPMarker + body[cut+2:]
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
