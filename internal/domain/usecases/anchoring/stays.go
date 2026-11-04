package anchoring

import (
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring/selector"
)

type scene struct {
	graph   entities.Graph
	decls   []entities.Decl
	one     kept
	newer   map[entities.DeclKey]bool
	owned   map[entities.DeclKey]bool
	holders []entry
	reader  Reader
	memo    *selector.Memo
	atoms   map[string][]entities.DeclKey
	modules map[string]bool
}

func newScene(graph entities.Graph, reader Reader) scene {
	atoms := map[string][]entities.DeclKey{}
	decls := graph.Decls()

	for _, decl := range decls {
		if decl.Atom != "" {
			atoms[decl.Atom] = append(atoms[decl.Atom], decl.Key)
		}
	}

	return scene{
		graph: graph, decls: decls, atoms: atoms, reader: reader,
		modules: selector.ModuleDirs(graph), memo: selector.NewMemo(reader),
	}
}

func holdersBefore(chosen []kept, at int) []entry {
	var out []entry

	own := keySet(chosen[at : at+1])
	newer := map[entities.DeclKey]bool{}

	for idx, one := range chosen {
		for _, placed := range one.entries {
			if idx > at && (own[placed.key] || newer[placed.key]) {
				continue
			}

			if idx > at {
				newer[placed.key] = true
			}

			out = append(out, placed)
		}
	}

	return out
}

func (view scene) mentions(
	key entities.DeclKey,
	moving map[entities.DeclKey]bool,
	index map[entities.DeclKey]int,
	slot int,
) bool {
	name := key.Name
	if cut := strings.LastIndex(name, "."); cut >= 0 {
		recv := entities.DeclKey{File: key.File, Kind: key.Kind, Name: name[:cut]}
		for owner := range moving {
			if owner.File == recv.File && owner.Name == recv.Name {
				return false
			}
		}

		name = name[cut+1:]
	}

	if name == "" {
		return false
	}

	for _, holder := range view.holders {
		if moving[holder.key] || index[holder.key] >= slot {
			continue
		}

		if wordIn(holder.preimage, name) {
			return true
		}
	}

	return false
}

func (view scene) splitsAtom(
	atom string,
	moving map[entities.DeclKey]bool,
	index map[entities.DeclKey]int,
) bool {
	for _, mate := range view.atoms[atom] {
		if _, isPlaced := index[mate]; isPlaced && !moving[mate] {
			return true
		}
	}

	return false
}

func (view scene) holds(
	key entities.DeclKey,
	moving map[entities.DeclKey]bool,
	index map[entities.DeclKey]int,
	slot int,
) string {
	decl, _ := view.graph.Decl(key)

	if view.splitsAtom(decl.Atom, moving, index) {
		return "an atom mate stays"
	}

	// A pre-image is held text, so the final tree's graph has no edge for what it reads.
	if reason := view.preimages(key, moving, index, slot); reason != "" {
		return reason
	}

	for _, edge := range view.graph.Needs(key) {
		at, isPlaced := index[edge.Provider]
		if isPlaced && at > slot && !moving[edge.Provider] &&
			!entities.FillsInterface(view.graph, edge) {
			return "a provider is born after the anchor"
		}
	}

	for _, edge := range view.graph.Consumers(key) {
		at, isPlaced := index[edge.Dependent]
		if isPlaced && at < slot && !moving[edge.Dependent] && !view.newer[edge.Dependent] &&
			!entities.FillsInterface(view.graph, edge) {
			return "a consumer born earlier needs it"
		}
	}

	return ""
}

func (view scene) preimages(
	key entities.DeclKey,
	moving map[entities.DeclKey]bool,
	index map[entities.DeclKey]int,
	slot int,
) string {
	if view.mentions(key, moving, index, slot) {
		return "a pre-image still names it"
	}

	for _, holder := range view.holders {
		if holder.key != key {
			continue
		}

		if hit := view.readsAfter(holder, index, moving, slot); hit != "" {
			return "its own pre-image may read " + hit
		}
	}

	return ""
}

func (view scene) reads(placed entry, name string) bool {
	if view.reader != nil {
		names, isParsed := view.reader.Reads(placed.key.File, placed.preimage)
		if isParsed {
			return slices.Contains(names, name)
		}
	}

	return wordIn(placed.preimage, name)
}
