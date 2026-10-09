package dart

import (
	"context"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// provider is the declaration a name reaches, and the export directives it
// passes through on the way: each of them has to exist too, or the barrel
// the reader imports does not offer the name yet.
type provider struct {
	key entities.DeclKey
	via []entities.DeclKey
}

// namespace is what a library offers: names reached as identifiers, the
// members of its extensions, reached only as "value.member", and the exports
// of a library outside the tree, whose names no one here can list.
type namespace struct {
	names   map[string]provider
	members map[string]provider
	open    map[entities.DeclKey]provider
}

func newNamespace() namespace {
	return namespace{
		names:   map[string]provider{},
		members: map[string]provider{},
		open:    map[entities.DeclKey]provider{},
	}
}

func (n namespace) add(name string, found provider, isMember bool) {
	target := n.names
	if isMember {
		target = n.members
	}

	if _, taken := target[name]; !taken {
		target[name] = found
	}
}

// declare adds what f declares itself. Private names are kept only for the
// library's own scope; an importer never sees them.
func (n namespace) declare(f parsedFile, isPrivateKept bool) {
	for _, it := range f.decls() {
		if it.kind != itemDecl {
			continue
		}

		for _, name := range it.names {
			if isPrivateKept || !strings.HasPrefix(name, "_") {
				n.add(name, provider{key: it.key}, false)
			}
		}

		for _, member := range it.members {
			n.add(member, provider{key: it.key}, true)
		}
	}
}

// exported is the namespace an importer of file sees: its public names, its
// parts', and every name its exports pass on.
func (w *workspace) exported(ctx context.Context, file string) (namespace, error) {
	cached, found := w.exports[file]

	if found {
		return cached, nil
	}

	out := newNamespace()

	// A placeholder breaks an export cycle: the second visit sees what the
	// first has gathered so far instead of recursing forever.
	w.exports[file] = out

	parsed, isOwned, err := w.file(ctx, file)
	if err != nil || !isOwned {
		return out, err
	}

	if err = w.declareWithParts(ctx, parsed, out, false); err != nil {
		return out, err
	}

	for _, it := range parsed.items {
		if it.kind != itemExport {
			continue
		}

		if err = w.reexport(ctx, parsed.path, it, out); err != nil {
			return out, err
		}
	}

	return out, nil
}

func (w *workspace) reexport(
	ctx context.Context,
	from string,
	export parsedItem,
	out namespace,
) error {
	target, err := w.resolve(ctx, from, export.directive.uri)
	if err != nil {
		return err
	}

	if target == "" {
		out.open[export.key] = provider{key: export.key}

		return nil
	}

	passed, err := w.exported(ctx, target)
	if err != nil {
		return err
	}

	for name, found := range filtered(passed.names, export.directive) {
		out.add(name, through(export.key, found), false)
	}

	for name, found := range passed.members {
		out.add(name, through(export.key, found), true)
	}

	for key, found := range openThrough(passed, export.directive) {
		if _, taken := out.open[key]; !taken {
			out.open[key] = through(export.key, found)
		}
	}

	return nil
}

func through(export entities.DeclKey, found provider) provider {
	return provider{key: found.key, via: append([]entities.DeclKey{export}, found.via...)}
}

// filtered applies a directive's show and hide lists to names.
func filtered(names map[string]provider, rule directive) map[string]provider {
	out := make(map[string]provider, len(names))

	for name, found := range names {
		if len(rule.show) > 0 && !slices.Contains(rule.show, name) {
			continue
		}

		if slices.Contains(rule.hide, name) {
			continue
		}

		out[name] = found
	}

	return out
}
