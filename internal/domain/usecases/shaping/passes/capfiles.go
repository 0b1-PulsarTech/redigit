package passes

import (
	"cmp"
	"path"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/shaping"
)

type capFile struct {
	name  string
	keys  []entities.DeclKey
	lines int
	test  bool
}

func filesByHandOrder(graph entities.Graph, keys []entities.DeclKey) []capFile {
	byName := map[string]*capFile{}

	for _, info := range infosOf(graph, keys) {
		file, seen := byName[info.key.File]
		if !seen {
			file = &capFile{name: info.key.File, test: true}
			byName[info.key.File] = file
		}

		file.keys = append(file.keys, info.key)
		file.lines += info.lines
		file.test = file.test && info.test
	}

	files := make([]capFile, 0, len(byName))

	// Only test files are offered: a production file leaving after a reader the hard edges do
	// not name (a soft implements edge) would turn the rows between them red.
	for _, file := range byName {
		if file.test {
			files = append(files, *file)
		}
	}

	slices.SortFunc(files, func(first, second capFile) int {
		return cmp.Or(cmp.Compare(second.lines, first.lines), cmp.Compare(first.name, second.name))
	})

	return files
}

func withConsumers(
	draft *shaping.Draft, held, seeds []entities.DeclKey,
) []entities.DeclKey {
	inRow := make(map[entities.DeclKey]bool, len(held))
	atoms := map[string][]entities.DeclKey{}

	for _, key := range held {
		inRow[key] = true

		if decl, _ := draft.Graph().Decl(key); decl.Atom != "" {
			atoms[decl.Atom] = append(atoms[decl.Atom], key)
		}
	}

	joined := make(map[entities.DeclKey]bool, len(seeds))
	pending := slices.Clone(seeds)

	for len(pending) > 0 {
		key := pending[len(pending)-1]
		pending = pending[:len(pending)-1]

		if joined[key] {
			continue
		}

		joined[key] = true

		decl, _ := draft.Graph().Decl(key)
		pending = append(pending, atoms[decl.Atom]...)

		for _, edge := range draft.Graph().Consumers(key) {
			// A var _ Doer = fake{} reads the method without a hard edge, and must not be left behind.
			isBound := entities.IsHard(edge.Reason) || edge.Reason == entities.ReasonImplements
			if isBound && inRow[edge.Dependent] {
				pending = append(pending, edge.Dependent)
			}
		}
	}

	// The row's own order keeps what leaves readable and the result deterministic.
	return slices.DeleteFunc(slices.Clone(held), func(key entities.DeclKey) bool {
		return !joined[key]
	})
}

func handedOff(
	graph entities.Graph, source entities.PlannedCommit, keys []entities.DeclKey,
) entities.PlannedCommit {
	commit := source
	commit.Decls = keys
	commit.Subject = titleOf(graph, commit)
	commit.Reason = "a row past the line cap hands whole files to the row after it"

	return commit
}

func titleOf(graph entities.Graph, commit entities.PlannedCommit) string {
	kind := "feat"

	switch {
	case allTest(infosOf(graph, commit.Decls)):
		kind = "test"
	case commit.Shape == entities.ShapeLayer:
		kind = "chore"
	}

	// The scope is what the planner said for the row, else what it would say for its feature.
	scope := path.Base(commit.Feature)

	_, inner, isScoped := strings.Cut(commit.Subject, "(")
	if isScoped && entities.IsDraft(commit.Subject) {
		scope, _, _ = strings.Cut(inner, ")")
	}

	if scope == "" || scope == "." {
		scope = "shared"
	}

	return entities.Redraft(entities.DraftMarker+kind+"("+scope+"): ", commit.Decls)
}
