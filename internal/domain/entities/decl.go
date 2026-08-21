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

// Location addresses a point in a file the way a language server does. Char is
// an offset within the line whose unit depends on the encoding negotiated with
// the server, so it is never treated as a byte offset.
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
	Base     Position
}
