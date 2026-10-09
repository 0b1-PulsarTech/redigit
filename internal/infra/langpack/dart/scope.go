package dart

import (
	"context"
	"slices"
)

// importScope is one import directive and what it offers. target is "" for a
// library outside the tree, whose names are unknown.
type importScope struct {
	directive directive
	target    string
	offer     namespace
}

// scope is what a library's code can name: its own declarations first, since
// they shadow every import, then each import in order.
type scope struct {
	own     namespace
	imports []importScope
}

// ownLibrary marks a reference resolved by the library itself, not an import.
const ownLibrary = -1

func (w *workspace) scopeOf(ctx context.Context, f parsedFile) (scope, error) {
	out := scope{own: newNamespace()}

	if err := w.declareWithParts(ctx, f, out.own, true); err != nil {
		return scope{}, err
	}

	for _, it := range f.items {
		if it.kind != itemImport {
			continue
		}

		target, err := w.resolve(ctx, f.path, it.directive.uri)
		if err != nil {
			return scope{}, err
		}

		offer := newNamespace()

		if target != "" {
			exported, exportErr := w.exported(ctx, target)
			if exportErr != nil {
				return scope{}, exportErr
			}

			offer = namespace{
				names:   filtered(exported.names, it.directive),
				members: exported.members,
				open:    openThrough(exported, it.directive),
			}
		}

		out.imports = append(
			out.imports,
			importScope{directive: it.directive, target: target, offer: offer},
		)
	}

	return out, nil
}

func (s scope) hasPrefix(name string) bool {
	return slices.ContainsFunc(s.imports, func(each importScope) bool {
		return each.directive.prefix == name
	})
}

// lookup resolves an unprefixed identifier: to the library's own declaration,
// to every import offering it, to an extension member called on an implicit
// receiver, or else to every import whose exports leave the tree. Two imports may offer one name - a
// barrel and the file behind it - and the analyzer counts both as used, so
// both have to survive a partial render and both have to exist first. A name
// an outside library shows by name uses that import though its declaration is
// unknown.
func (s scope) lookup(name string) []reference {
	if found, isOwn := s.own.names[name]; isOwn {
		return []reference{{provider: found, through: ownLibrary}}
	}

	var out []reference

	for index, each := range s.imports {
		if each.directive.prefix != "" {
			continue
		}

		if found, offered := each.offer.names[name]; offered {
			out = append(out, reference{provider: found, through: index})
		} else if each.target == "" && slices.Contains(each.directive.show, name) {
			out = append(out, reference{through: index})
		}
	}

	if len(out) > 0 {
		return out
	}

	if members := s.member(name); len(members) > 0 {
		return members
	}

	return s.unlisted(name, "")
}

// prefixed resolves "prefix.name". An outside library sharing the prefix may
// be the one offering the name, which nothing here can tell, so it counts as
// used whenever the prefix is.
func (s scope) prefixed(prefix, name string) []reference {
	var out []reference

	for index, each := range s.imports {
		if each.directive.prefix != prefix {
			continue
		}

		if found, offered := each.offer.names[name]; offered {
			out = append(out, reference{provider: found, through: index})
		} else if each.target == "" {
			out = append(out, reference{through: index})
		} else {
			out = append(out, openReferences(each, index)...)
		}
	}

	return out
}

func (s scope) member(name string) []reference {
	if found, isOwn := s.own.members[name]; isOwn {
		return []reference{{provider: found, through: ownLibrary}}
	}

	var out []reference

	for index, each := range s.imports {
		if found, offered := each.offer.members[name]; offered {
			out = append(out, reference{provider: found, through: index})
		}
	}

	return out
}
