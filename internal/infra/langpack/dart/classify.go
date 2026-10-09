package dart

import (
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type itemKind uint8

const (
	itemFrame itemKind = iota + 1
	itemImport
	itemExport
	itemPart
	itemDecl
)

// shape is what one item declares. names holds every name a variable list
// binds; nameAt indexes the name token in the item's own tokens, or is -1.
type shape struct {
	kind     itemKind
	declKind entities.DeclKind
	name     string
	names    []string
	nameAt   int
	members  []string
}

var classModifiers = []string{
	"abstract",
	"base",
	"final",
	"interface",
	"sealed",
	"mixin",
	"augment",
}

var valueModifiers = []string{"external", "static", "late", "const", "final", "var", "augment"}

func classify(toks []token) shape {
	at := skipMetadata(toks, 0)
	if at >= len(toks) {
		return shape{kind: itemFrame, nameAt: -1}
	}

	switch toks[at].text {
	case "library":
		return shape{kind: itemFrame, nameAt: -1}
	case "part":
		if at+1 < len(toks) && toks[at+1].text == "of" {
			return shape{kind: itemFrame, nameAt: -1}
		}

		return shape{kind: itemPart, nameAt: -1}
	case "import":
		return shape{kind: itemImport, nameAt: -1}
	case "export":
		return shape{kind: itemExport, declKind: entities.KindOther, nameAt: -1}
	case "enum":
		return named(toks, at+1, entities.KindType)
	case "typedef":
		return typedefShape(toks, at)
	case "extension":
		return extensionShape(toks, at)
	}

	if found, isType := classShape(toks, at); isType {
		return found
	}

	return memberShape(toks, at)
}

func named(toks []token, at int, kind entities.DeclKind) shape {
	if at >= len(toks) || toks[at].kind != tokIdent {
		return shape{kind: itemDecl, declKind: entities.KindOther, nameAt: -1}
	}

	return shape{
		kind:     itemDecl,
		declKind: kind,
		name:     toks[at].text,
		names:    []string{toks[at].text},
		nameAt:   at,
	}
}

func classShape(toks []token, at int) (shape, bool) {
	cursor := at
	for cursor < len(toks) && slices.Contains(classModifiers, toks[cursor].text) {
		cursor++
	}

	switch {
	case cursor < len(toks) && toks[cursor].text == "class":
		return named(toks, cursor+1, entities.KindType), true
	case cursor > at && toks[cursor-1].text == "mixin":
		return named(toks, cursor, entities.KindType), true
	default:
		return shape{}, false
	}
}

// typedefShape reads both forms: "typedef Name<T> = ..." and the older
// "typedef void Name(int x);", whose name is the identifier before "(".
func typedefShape(toks []token, at int) shape {
	if at+2 < len(toks) && (toks[at+2].text == "=" || toks[at+2].text == "<") {
		return named(toks, at+1, entities.KindType)
	}

	for index := at + 1; index < len(toks); index++ {
		if toks[index].text == "(" {
			return named(toks, identBefore(toks, index), entities.KindType)
		}
	}

	return named(toks, at+1, entities.KindType)
}

// extensionShape names an unnamed extension by what it extends, which is
// stable across revisions where a line number would not be.
func extensionShape(toks []token, at int) shape {
	if at+1 >= len(toks) {
		return named(toks, at+1, entities.KindType)
	}

	second := at + 1

	switch toks[second].text {
	case "type":
		cursor := second + 1
		if cursor < len(toks) && toks[cursor].text == "const" {
			cursor++
		}

		return named(toks, cursor, entities.KindType)
	case "on":
		var onType []string

		for index := second + 1; index < len(toks) && toks[index].text != "{"; index++ {
			onType = append(onType, toks[index].text)
		}

		found := shape{
			kind:     itemDecl,
			declKind: entities.KindType,
			name:     "extension on " + strings.Join(onType, ""),
			nameAt:   -1,
		}
		found.members = extensionMembers(toks)

		return found
	default:
		found := named(toks, at+1, entities.KindType)
		found.members = extensionMembers(toks)

		return found
	}
}
