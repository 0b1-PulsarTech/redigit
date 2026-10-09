package config

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// A README with a module edge and no reader was born at 96% of a history. In
// the header's atom it is born with the go.mod.
func TestModuleFilesRuleJoinsTheHeadersAtom(t *testing.T) {
	t.Parallel()

	header := piece("apps/lead/go.mod", "example.com/app", entities.Span{})
	header.Atom = "workspace:apps/lead"
	readme := piece("apps/lead/README.md", "<file>", entities.Span{})
	nested := piece("apps/lead/svc/notes.md", "<file>", entities.Span{})
	otherReadme := piece("apps/other/README.md", "<file>", entities.Span{})
	decls := []entities.Decl{
		header,
		readme,
		nested,
		otherReadme,
		goDecl("apps/lead/svc/svc.go", "Serve"),
	}

	atoms, err := (moduleFilesRule{}).Atoms(t.Context(), entities.Snapshot{}, decls)
	if err != nil {
		t.Fatalf("Atoms() = %v", err)
	}

	if atoms[readme.Key] != header.Atom {
		t.Errorf("README atom = %q, want %q", atoms[readme.Key], header.Atom)
	}

	if len(atoms) != 1 {
		t.Errorf("atoms = %v, want only the module's README", atoms)
	}

	anchors, err := (moduleFilesRule{}).Anchors(t.Context(), entities.Snapshot{}, decls)
	if err != nil {
		t.Fatalf("Anchors() = %v", err)
	}

	if len(anchors) != 1 || anchors[0].Decl != readme.Key || anchors[0].To != header.Key {
		t.Errorf("anchors = %v, want the README lowered to its header", anchors)
	}
}

func TestRulePackUnionsTheAtomsOfItsRules(t *testing.T) {
	t.Parallel()

	header := piece("apps/lead/go.mod", "example.com/app", entities.Span{})
	header.Atom = "workspace:apps/lead"
	readme := piece("apps/lead/README.md", "<file>", entities.Span{})
	decls := []entities.Decl{header, readme, goDecl("apps/lead/svc/svc.go", "Serve")}

	atoms, err := NewRulePack().Atoms(t.Context(), entities.Snapshot{}, decls)
	if err != nil {
		t.Fatalf("Atoms() = %v", err)
	}

	if atoms[readme.Key] != header.Atom {
		t.Errorf("atoms = %v, want the README in the header's atom", atoms)
	}
}
