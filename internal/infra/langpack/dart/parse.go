package dart

import (
	"strconv"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// directive is an import, export, or part. uri is the first URI it names; a
// configurable import's alternatives are left out, since only one is read.
type directive struct {
	uri    string
	prefix string
	show   []string
	hide   []string
}

// parsedItem is an item with what it declares. key is set for an export and
// for a declaration, the two kinds a commit can keep or drop.
type parsedItem struct {
	item
	shape

	key       entities.DeclKey
	directive directive
}

// parsedFile is one Dart file read on its own, which is all Declarations and
// Render need; what its imports provide is the workspace's question.
type parsedFile struct {
	path   string
	source string
	toks   []token
	items  []parsedItem
}

func parseFile(path, source string) parsedFile {
	toks := lex(source)
	file := parsedFile{path: path, source: source, toks: toks}
	seen := map[entities.DeclKey]int{}

	for _, it := range splitItems(source, toks) {
		own := it.tokens(toks)
		parsed := parsedItem{item: it, shape: classify(own)}

		switch parsed.kind {
		case itemImport, itemExport, itemPart:
			parsed.directive = parseDirective(own)
		}

		if parsed.kind == itemExport {
			parsed.name = "export " + parsed.directive.uri
		}

		if parsed.kind == itemExport || parsed.kind == itemDecl {
			parsed.key = uniqueKey(seen, path, parsed.shape, len(file.items))
		}

		file.items = append(file.items, parsed)
	}

	return file
}

// uniqueKey keeps two items from sharing a key, which only invalid code or an
// unreadable construct produces; dropping one would drop its text too.
func uniqueKey(
	seen map[entities.DeclKey]int,
	path string,
	found shape,
	position int,
) entities.DeclKey {
	name := found.name
	if name == "" {
		name = "<item " + strconv.Itoa(position) + ">"
	}

	key := entities.DeclKey{File: path, Kind: found.declKind, Name: name}
	seen[key]++

	if count := seen[key]; count > 1 {
		key.Name += "#" + strconv.Itoa(count)
	}

	return key
}

func parseDirective(toks []token) directive {
	var (
		found  directive
		clause *[]string
	)

	for index := 1; index < len(toks); index++ {
		tok := toks[index]

		switch {
		case tok.kind == tokString && found.uri == "":
			found.uri = tok.value
		case tok.text == "as" && index+1 < len(toks):
			found.prefix = toks[index+1].text
			index++
		case tok.text == "show":
			clause = &found.show
		case tok.text == "hide":
			clause = &found.hide
		case tok.kind == tokIdent && clause != nil:
			*clause = append(*clause, tok.text)
		}
	}

	return found
}

func (f parsedFile) decls() []parsedItem {
	var out []parsedItem

	for _, it := range f.items {
		if !it.key.IsZero() {
			out = append(out, it)
		}
	}

	return out
}
