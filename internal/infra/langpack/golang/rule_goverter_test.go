package golang

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestGoverterRuleEdges(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := "convert"

	writeFile(t, root, filepath.Join(dir, "thing_conv.go"), `package convert

// goverter:converter
type Converter interface {
	ToDTO(Thing) ThingDTO
}
`)
	writeFile(t, root, filepath.Join(dir, "converter_gen.go"), `package convert

func (c *ConverterImpl) ToDTO(t Thing) ThingDTO { return ThingDTO{} }
`)
	writeFile(t, root, filepath.Join(dir, "unrelated.go"), `package convert

func Unrelated() {}
`)

	snap := entities.Snapshot{Root: root}
	syntax := NewSyntax()

	decls := declareAll(t, syntax, root, []string{
		filepath.Join(dir, "thing_conv.go"),
		filepath.Join(dir, "converter_gen.go"),
		filepath.Join(dir, "unrelated.go"),
	})

	edges, err := Goverter{}.Edges(context.Background(), snap, decls)
	if err != nil {
		t.Fatalf("Edges() error = %v", err)
	}

	if len(edges) != 1 {
		t.Fatalf("got %d edges, want 1: %+v", len(edges), edges)
	}

	edge := edges[0]
	if edge.Provider.Name != "Converter" ||
		edge.Provider.File != filepath.Join(dir, "thing_conv.go") {
		t.Fatalf("Provider = %+v, want the goverter-directive interface", edge.Provider)
	}

	if edge.Dependent.File != filepath.Join(dir, "converter_gen.go") {
		t.Fatalf("Dependent = %+v, want the generated file's declaration", edge.Dependent)
	}

	if edge.Reason != entities.ReasonGeneratedBy {
		t.Fatalf("Reason = %v, want %v", edge.Reason, entities.ReasonGeneratedBy)
	}
}

func TestGoverterRuleNoDirectiveNoEdge(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := "convert"

	writeFile(t, root, filepath.Join(dir, "thing_conv.go"), `package convert

// plain interface, no goverter directive
type Converter interface {
	ToDTO(Thing) ThingDTO
}
`)
	writeFile(t, root, filepath.Join(dir, "converter_gen.go"), `package convert

func (c *ConverterImpl) ToDTO(t Thing) ThingDTO { return ThingDTO{} }
`)

	snap := entities.Snapshot{Root: root}
	syntax := NewSyntax()

	decls := declareAll(t, syntax, root, []string{
		filepath.Join(dir, "thing_conv.go"),
		filepath.Join(dir, "converter_gen.go"),
	})

	edges, err := Goverter{}.Edges(context.Background(), snap, decls)
	if err != nil {
		t.Fatalf("Edges() error = %v", err)
	}

	if len(edges) != 0 {
		t.Fatalf("got %d edges, want 0 without a goverter directive: %+v", len(edges), edges)
	}
}
