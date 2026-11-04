package slicing

import (
	"strconv"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/syntax"
)

func (cut bodyCut) sliceElements() indexing.Sliced {
	joined := newGroups(len(cut.elements) + 1)
	head := len(cut.elements)
	joined.join(head, 0)

	out := indexing.Sliced{Head: cut.head(), Members: cut.members()}

	for index := range out.Members {
		out.Edges = append(out.Edges, entities.Edge{
			Dependent: out.Members[index].Key,
			Provider:  cut.fn.Key,
			Reason:    entities.ReasonBody,
		})
		out.Members[index].Atom = cut.elementAtom(joined, index)
	}

	out.Head.Atom = cut.elementAtom(joined, head)

	return out
}

func (cut bodyCut) elementAtom(joined groups, index int) string {
	root := joined.find(index)
	if joined.size(root) < syntax.MinGroup {
		return ""
	}

	return cut.fn.Key.File + ":" + cut.fn.Key.Name + "#atom" + strconv.Itoa(root)
}
