package cli

import (
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/auditing"
)

// measureDTO carries a number and, always, what that number did not look at.
// Excluded is not omitempty on purpose: a measure printed without its blind
// spot is the shape of every bad number this tool exists to avoid.
type measureDTO struct {
	Name     string   `json:"name"`
	Value    int      `json:"value"`
	Total    int      `json:"total"`
	Percent  int      `json:"percent"`
	Excluded []string `json:"excluded"`
}

type driftDTO struct {
	Decl      string `json:"decl"`
	BornAt    int    `json:"born_at"`
	FirstRead int    `json:"first_read"`
	Gap       int    `json:"gap"`
	Reader    string `json:"reader"`
	Reason    string `json:"reason"`
}

type cycleDTO struct {
	Members []string `json:"members"`
	Reasons []string `json:"reasons"`
}

// auditDTO is what `redigit audit` prints. Integrity and shape stay apart:
// a rebuild can replay almost perfectly while grouping badly, and one merged
// number would hide exactly that.
// unseenDTO is one file this run could not read, and the constraint that hid
// it. It is not omitempty: an empty list is the claim that nothing was hidden,
// and a reader must be able to tell that from a tool too old to report it.
type unseenDTO struct {
	File       string   `json:"file"`
	Constraint string   `json:"constraint"`
	Tags       []string `json:"tags"`
}

type auditDTO struct {
	Repo         string      `json:"repo"`
	Rev          string      `json:"rev"`
	BuildTags    []string    `json:"build_tags"`
	Declarations int         `json:"declarations"`
	Edges        int         `json:"edges"`
	Rounds       int         `json:"rounds"`
	Unseen       []unseenDTO `json:"unseen"`
	// Unclaimed is not omitempty either, and for the same reason Unseen
	// isn't: a path no language pack claims is never even read, so an empty
	// list here is the claim that every final-tree path was claimed, not
	// that this run is too old to say so.
	Unclaimed []string     `json:"unclaimed"`
	Integrity []measureDTO `json:"integrity"`
	Shape     []measureDTO `json:"shape"`
	Cycles    []cycleDTO   `json:"cycles"`
	Drift     []driftDTO   `json:"drift"`
}

type commitDTO struct {
	Position int      `json:"position"`
	Subject  string   `json:"subject"`
	Shape    string   `json:"shape"`
	Feature  string   `json:"feature,omitempty"`
	Reason   string   `json:"reason,omitempty"`
	Decls    []string `json:"decls"`
}

type planDTO struct {
	Repo     string      `json:"repo"`
	Rev      string      `json:"rev"`
	Commits  []commitDTO `json:"commits"`
	Unplaced []string    `json:"unplaced"`
}

type declDTO struct {
	Key      string `json:"key"`
	Unit     string `json:"unit"`
	IsTest   bool   `json:"is_test"`
	Position int    `json:"position"`
}

type edgeDTO struct {
	Dependent string `json:"dependent"`
	Provider  string `json:"provider"`
	Reason    string `json:"reason"`
}

type graphDTO struct {
	Repo  string    `json:"repo"`
	Rev   string    `json:"rev"`
	Decls []declDTO `json:"decls"`
	Edges []edgeDTO `json:"edges"`
}

func toUnseen(in []auditing.Unseen) []unseenDTO {
	out := make([]unseenDTO, 0, len(in))

	for _, unseen := range in {
		tags := unseen.Tags
		if tags == nil {
			tags = []string{}
		}

		out = append(out, unseenDTO{File: unseen.File, Constraint: unseen.Constraint, Tags: tags})
	}

	return out
}

func toUnclaimed(in []auditing.Unclaimed) []string {
	out := make([]string, 0, len(in))

	for _, unclaimed := range in {
		out = append(out, unclaimed.File)
	}

	return out
}

func toMeasures(in []entities.Measure) []measureDTO {
	out := make([]measureDTO, 0, len(in))
	for _, measure := range in {
		out = append(out, measureDTO{
			Name:     measure.Name,
			Value:    measure.Value,
			Total:    measure.Total,
			Percent:  measure.Percent(),
			Excluded: measure.Excluded,
		})
	}

	return out
}

func toDrift(in []auditing.Drift) []driftDTO {
	out := make([]driftDTO, 0, len(in))
	for _, drift := range in {
		out = append(out, driftDTO{
			Decl:      drift.Decl.String(),
			BornAt:    int(drift.BornAt),
			FirstRead: int(drift.FirstRead),
			Gap:       drift.Gap(),
			Reader:    drift.Reader.String(),
			Reason:    string(drift.Reason),
		})
	}

	return out
}

func toCycles(in []entities.Cycle) []cycleDTO {
	out := make([]cycleDTO, 0, len(in))

	for _, cycle := range in {
		members := make([]string, 0, len(cycle.Members))
		for _, key := range cycle.Members {
			members = append(members, key.String())
		}

		reasons := make([]string, 0, len(cycle.Reasons))
		for _, reason := range cycle.Reasons {
			reasons = append(reasons, string(reason))
		}

		out = append(out, cycleDTO{Members: members, Reasons: reasons})
	}

	return out
}

func toKeys(in []entities.DeclKey) []string {
	out := make([]string, 0, len(in))
	for _, key := range in {
		out = append(out, key.String())
	}

	return out
}
