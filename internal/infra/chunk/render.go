package chunk

import (
	"fmt"

	"golang.org/x/mod/modfile"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// Renderer writes a file back holding only the pieces a commit keeps, for a
// file whose BLOCK structure concatenation cannot rebuild: a go.mod's require
// and replace lines sit inside headers that belong to no single line, so a
// subset concatenated puts a replace inside the require block and the file
// stops parsing. A round trip over the FULL set never catches that.
type Renderer interface {
	Render(source []byte, keep []entities.DeclKey) ([]byte, error)
}

var (
	_ Renderer = GoMod{}
	_ Renderer = GoWork{}
)

// Render rebuilds a go.mod holding only the kept pieces, using the same parser
// and formatter the go command uses. Dropping lines textually is what produced
// the invalid file; asking modfile to drop them and format the result produces
// what `go mod edit` would.
func (GoMod) Render(source []byte, keep []entities.DeclKey) ([]byte, error) {
	if len(keep) == 0 {
		return nil, nil
	}

	file, err := modfile.Parse("go.mod", source, nil)
	if err != nil {
		return nil, fmt.Errorf("parse go.mod: %w", err)
	}

	kept := namesOf(keep)

	for _, require := range file.Require {
		if !kept[requireDirective+" "+require.Mod.Path] {
			_ = file.DropRequire(require.Mod.Path)
		}
	}

	for _, replace := range file.Replace {
		if !kept[replaceDirective+" "+replace.Old.Path] {
			_ = file.DropReplace(replace.Old.Path, replace.Old.Version)
		}
	}

	file.Cleanup()

	out, err := file.Format()
	if err != nil {
		return nil, fmt.Errorf("format go.mod: %w", err)
	}

	return out, nil
}

// Render rebuilds a go.work holding only the kept use lines.
func (GoWork) Render(source []byte, keep []entities.DeclKey) ([]byte, error) {
	if len(keep) == 0 {
		return nil, nil
	}

	file, err := modfile.ParseWork("go.work", source, nil)
	if err != nil {
		return nil, fmt.Errorf("parse go.work: %w", err)
	}

	kept := namesOf(keep)

	for _, use := range file.Use {
		if !kept[useDirective+" "+use.Path] {
			_ = file.DropUse(use.Path)
		}
	}

	file.Cleanup()

	return modfile.Format(file.Syntax), nil
}

func namesOf(keep []entities.DeclKey) map[string]bool {
	out := make(map[string]bool, len(keep))
	for _, key := range keep {
		out[key.Name] = true
	}

	return out
}

// The directive keywords, named once so the chunkers that write a piece's name
// and the renderers that read it back can never drift apart on a typo.
const (
	requireDirective = "require"
	replaceDirective = "replace"
	useDirective     = "use"
)
