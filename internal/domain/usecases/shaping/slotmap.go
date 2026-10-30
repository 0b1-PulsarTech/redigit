package shaping

import (
	"maps"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type SlotMap struct {
	at    map[entities.DeclKey]int
	alias map[int]int
}

func NewSlotMap() *SlotMap {
	return &SlotMap{at: map[entities.DeclKey]int{}, alias: map[int]int{}}
}

func (slots *SlotMap) Place(key entities.DeclKey, slot int) { slots.at[key] = slot }

func (slots *SlotMap) Slot(key entities.DeclKey) (int, bool) {
	slot, found := slots.at[key]
	if !found {
		return 0, false
	}

	return slots.Resolve(slot), true
}

func (slots *SlotMap) Resolve(slot int) int {
	for {
		next, aliased := slots.alias[slot]
		if !aliased {
			return slot
		}

		slot = next
	}
}

func (slots *SlotMap) Merge(from, into int) {
	from, into = slots.Resolve(from), slots.Resolve(into)
	// Resolving both ends first keeps the alias table acyclic.
	if from != into {
		slots.alias[from] = into
	}
}

func (slots *SlotMap) Clone() *SlotMap {
	return &SlotMap{at: maps.Clone(slots.at), alias: maps.Clone(slots.alias)}
}
