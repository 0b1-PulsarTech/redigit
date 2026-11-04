package idiom

import (
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
)

//go:generate go tool -modfile=../../../../tools/go.mod mockgen -source=facts.go -destination=facts_mock_test.go -package=idiom

type Facts interface {
	Calls(site entities.DeclKey) []facts.Call
	CtorArgTypes(fn entities.DeclKey) ([]facts.TypeRef, error)
	Implementors(iface entities.DeclKey) []entities.DeclKey
	StmtCallsInto(stmt entities.DeclKey, pattern string) ([]facts.Call, error)
}
