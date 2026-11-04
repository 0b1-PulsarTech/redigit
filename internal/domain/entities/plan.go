package entities

import (
	"slices"
	"strings"
)

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

func IsWIPSubject(subject string) bool {
	_, _, isMarked := splitWIP(subject)

	return isMarked
}

func MarkWIP(subject string) string {
	head, rest, _ := splitWIP(subject)

	return head + WIPMarker + rest
}

func UnmarkWIP(subject string) string {
	head, rest, _ := splitWIP(subject)

	return head + rest
}

func splitWIP(subject string) (head, rest string, isMarked bool) {
	rest = subject
	if IsDraft(subject) {
		head, rest = DraftMarker, subject[len(DraftMarker):]
	}

	// A marker ahead of the type is how older code wrote it: read as marked, moved to the slot.
	for _, measure := range []func(string) int{emojiLength, typeLength} {
		rest = skipWIP(rest)
		size := measure(rest)
		head, rest = head+rest[:size], rest[size:]
	}

	rest = skipWIP(rest)

	return head, rest, len(head)+len(rest) < len(subject)
}

func skipWIP(text string) string {
	after, hasMarker := strings.CutPrefix(text, WIPMarker)
	if !hasMarker {
		return text
	}

	return skipWIP(after)
}

func emojiLength(text string) int {
	code, _, isCode := strings.Cut(text, ": ")
	if !isCode || len(code) < 2 || code[0] != ':' ||
		strings.Trim(code[1:], "abcdefghijklmnopqrstuvwxyz0123456789_+-") != "" {
		return 0
	}

	return len(code) + len(": ")
}

func typeLength(text string) int {
	line, _, _ := strings.Cut(text, "\n")
	rest := strings.TrimLeft(line, "abcdefghijklmnopqrstuvwxyz")

	if rest == line {
		return 0
	}

	if scope, isScoped := strings.CutPrefix(rest, "("); isScoped {
		inner, after, _ := strings.Cut(scope, ")")
		if inner == "" || strings.Contains(inner, "(") {
			return 0
		}

		rest = after
	}

	if !strings.HasPrefix(rest, ": ") {
		return 0
	}

	return len(line) - len(rest) + len(": ")
}
