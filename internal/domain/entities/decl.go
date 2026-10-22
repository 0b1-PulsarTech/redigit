package entities

import "strings"

// DeclKind is what a declaration declares. It is deliberately coarser than any
// one language's grammar: the ordering rules only ever ask whether something is
// a type, a callable, a bound value, or none of those.
type DeclKind string

const (
	KindFunc   DeclKind = "func"
	KindMethod DeclKind = "method"
	KindType   DeclKind = "type"
	KindValue  DeclKind = "value"
	KindOther  DeclKind = "other"
	// KindStmt is a piece of a function body: one statement of a function that
	// assembles the program, named "Function#index". The function itself stays a
	// KindFunc or KindMethod declaration holding the signature, and a statement
	// is born when what it registers exists.
	KindStmt DeclKind = "stmt"
)

// UnitID identifies the importable unit a declaration belongs to. A language
// server reports no such concept, so this is the directory plus the name the
// server gives the unit, which keeps an external test package distinct from the
// package it tests.
type UnitID string

// Dir returns the directory part of the unit, which is what file-level rules
// (a test beside its subject, a sibling generator directive) key on.
func (u UnitID) Dir() string {
	dir, _, found := strings.Cut(string(u), "#")
	if !found {
		return string(u)
	}

	return dir
}

// DeclKey identifies one declaration for the whole run. The file is part of the
// identity because the same name may be declared in two files of one unit, and
// because a declaration is rendered back into the file it came from.
type DeclKey struct {
	File string
	Kind DeclKind
	Name string
}

// String is the stable text form used in reports, caches, and plan files.
func (k DeclKey) String() string {
	return k.File + ":" + string(k.Kind) + ":" + k.Name
}

// IsZero reports whether the key names nothing, which is how a lookup miss is
// returned without a second boolean.
func (k DeclKey) IsZero() bool {
	return k.File == "" && k.Kind == "" && k.Name == ""
}

// Span is a byte range in a file's final content. Byte offsets, not lines: the
// renderer cuts text with them, and a line number cannot address a declaration
// that shares a line with another.
type Span struct {
	Start int
	End   int
}

// Location addresses a point in a file the way a language server does: Line is
// zero-based, and Char counts UTF-16 CODE UNITS from the start of the line, not
// bytes and not runes.
//
// The unit is stated here because it is invisible at the call site and wrong
// silently: a server asked at a byte column answers about a different character
// on any line holding a non-ASCII one, and reports nothing amiss. gopls never
// advertises positionEncoding at all, so there is no handshake to read it from.
// Whoever produces a Location converts; whoever consumes one may not assume
// bytes.
type Location struct {
	File string
	Line int
	Char int
}

// Decl is the unit the whole tool orders. Anchoring on the file was the mistake
// that cost the most in the hand rewrite this replaces: a file is born with three
// declarations and gains eight over months, so only a declaration can carry a
// position of its own.
type Decl struct {
	Key      DeclKey
	Unit     UnitID
	Span     Span
	Selector Location
	IsTest   bool
	// IsInterface marks a declaration whose implementations can be asked for.
	// The question is only meaningful in one direction: a type that satisfies
	// an interface depends on it, never the reverse. A language server answers
	// implementation queries BOTH ways, so asking about a concrete type and
	// keeping the answer inverts the relationship and makes a stable library
	// look like it depends on a test stub.
	//
	// A pack that leaves this false loses implementation edges rather than
	// inventing backwards ones, which is the safe direction to fail in.
	IsInterface bool
	// Atom groups declarations one tool wrote in a single pass, so they are
	// born together. It is a constraint and NOT a merge: they stay in the
	// graph, so what reads them still produces edges. Merging them instead
	// took their names out and a test landed 491 positions early.
	Atom string
	// GroupSize is how many declarations share this one's declaration block.
	// A constant is not judged alone when it belongs to a group: an enum of
	// two members is as indivisible as one of twenty-seven, and counting
	// siblings per FILE gets that wrong whenever a file holds two enums or
	// one enum plus a loose constant.
	GroupSize int
	// Lines is how many lines the declaration adds to its file, the blank
	// lines up to the next declaration included. The planner reads it to keep
	// a commit reviewable: a test package of three files can add 2700 lines
	// in one commit, which no reviewer reads.
	Lines int
	// Frame is how many lines of the file precede its first declaration
	// (package clause, imports, build tags). Whichever commit first touches
	// the file adds them, so the cost of a file is charged once per commit.
	Frame int
	// Generated marks a declaration of a file a tool wrote. Its lines do not
	// count toward the cap on hand-written lines, and when there are more of
	// them than the cap they stand in a commit of their own.
	Generated bool
}

// DefaultMaxAddedLines is the most hand-written lines one commit adds, shared
// by the planner that enforces it and the chunker that splits a text file
// which alone passes it.
const DefaultMaxAddedLines = 900

// DefaultMinCommitLines is the size below which a commit is a scrap: a reader
// opens it for a handful of lines that belong to the commit beside it.
const DefaultMinCommitLines = 15
