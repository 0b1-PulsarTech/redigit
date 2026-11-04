package subjecting

import (
	"maps"
	"slices"
	"strconv"
	"strings"
)

const IDPrefix = "id:"

type (
	Kind string

	Slot struct {
		Position int
		ID       string
	}

	Hand struct {
		Subject string
		Found   bool
	}

	Problem struct {
		Kind Kind
		Key  string
	}
)

const (
	KindStaleKey    Kind = "stale_key"
	KindUnknownID   Kind = "unknown_id"
	KindAmbiguousID Kind = "ambiguous_id"
	KindConflict    Kind = "key_conflict"
)

func Resolve(slots []Slot, subjects map[string]string) ([]Hand, []Problem) {
	// A slot reached by two keys, or a key reaching none, is a problem the caller rules on.
	byPosition := make(map[int]int, len(slots))
	byID := make(map[string]int, len(slots))

	for idx, slot := range slots {
		byPosition[slot.Position] = idx

		if slot.ID == "" {
			continue
		}

		if previous, seen := byID[slot.ID]; seen && previous >= 0 {
			byID[slot.ID] = -1
		} else if !seen {
			byID[slot.ID] = idx
		}
	}

	var problems []Problem

	hits := make(map[int][]string, len(subjects))

	for _, key := range slices.Sorted(maps.Keys(subjects)) {
		idx, problem := locate(key, byPosition, byID)
		if problem != "" {
			problems = append(problems, Problem{Kind: problem, Key: key})

			continue
		}

		hits[idx] = append(hits[idx], key)
	}

	hands := make([]Hand, len(slots))

	for idx := range slots {
		switch keys := hits[idx]; len(keys) {
		case 0:
		case 1:
			hands[idx] = Hand{Subject: subjects[keys[0]], Found: true}
		default:
			problems = append(problems, Problem{Kind: KindConflict, Key: strings.Join(keys, ",")})
		}
	}

	return hands, problems
}

func locate(key string, byPosition map[int]int, byID map[string]int) (int, Kind) {
	if hex, isID := strings.CutPrefix(key, IDPrefix); isID {
		idx, found := byID[hex]

		switch {
		case !found:
			return 0, KindUnknownID
		case idx < 0:
			return 0, KindAmbiguousID
		}

		return idx, ""
	}

	position, err := strconv.Atoi(key)
	if err != nil || strconv.Itoa(position) != key {
		return 0, KindStaleKey
	}

	idx, found := byPosition[position]
	if !found {
		return 0, KindStaleKey
	}

	return idx, ""
}
