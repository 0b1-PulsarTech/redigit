package linting

import (
	"context"
	"errors"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

const (
	MeasureSourceAdded    = "source_added_over"
	MeasureGeneratedMixed = "generated_mixed"
	MeasureRowAdded       = "rows_over"

	MeasureShellsBornEmpty   = "shells_born_empty"
	MeasureShellsWithoutTail = "shells_without_tail"
	MeasureStubGap           = "stub_gap"
	MeasureErrcheckSplit     = "errcheck_split"
	MeasureUnusedAtBirth     = "unused_at_birth"
)

var ErrMismatch = errors.New("the manifest does not match the graph")

type AssignForm string

const (
	FormNone   AssignForm = ""
	FormAssign AssignForm = "plain err ="
	FormDefine AssignForm = "err newly bound here"
	FormReuse  AssignForm = "err already bound, with other names defined"
)

type (
	File struct {
		Path  string
		Added int
		Kind  Kind
	}

	Row struct {
		Position int
		Subject  string
		Files    []File
	}

	Finding struct {
		Measure  string
		Position int
		Subject  string
		Value    int
		Limit    int
		Detail   string

		NotCounted bool
	}

	Measure struct {
		Name       string
		Definition string
		Limit      int
		Count      int
		Detail     string
		Excluded   []string
	}

	GraphTier struct {
		Commit       string
		CutFunctions int
	}

	Report struct {
		Rows     int
		Measured bool
		Findings []Finding
		Measures []Measure
		Graph    *GraphTier
	}

	Statement struct {
		Assign AssignForm
		Binds  bool
		Checks bool
	}

	Syntax interface {
		Statements(
			ctx context.Context,
			file string,
			spans map[entities.DeclKey]entities.Span,
		) (map[entities.DeclKey]Statement, error)
		Exempt(decl entities.Decl) string
	}

	Structure struct {
		Graph    entities.Graph
		Born     map[entities.DeclKey]int
		Subjects map[int]string
		Syntax   Syntax
	}
)

func Counted(findings []Finding) int {
	counted := 0

	for _, finding := range findings {
		if !finding.NotCounted {
			counted++
		}
	}

	return counted
}

func Deferred() []string {
	return []string{
		"errcheck_pairs_split", "shells_without_tail", "shells_born_empty",
		"unused_at_birth", "born_as_stubs_gap_over_20_rows",
	}
}
