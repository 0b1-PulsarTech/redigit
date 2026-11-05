package anchoring

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type (
	Delegate interface {
		Plan(graph entities.Graph, order entities.Ordering) (entities.Plan, error)
	}

	Revisions interface {
		Revision(ctx context.Context, rev string) (Revision, error)
		Blob(ctx context.Context, oid string) ([]byte, error)
	}

	Finals interface {
		Read(ctx context.Context, path string) ([]byte, error)
	}

	Reader interface {
		Reads(file, text string) ([]string, bool)
	}

	Importer interface {
		Needed(source []byte, text string) []string
	}

	Probe interface {
		Builds(ctx context.Context, edits []Edit) (Verdict, error)
	}
)

type (
	Revision struct {
		Subject string
		Files   []FileChange
	}

	FileChange struct {
		Path, OldPath, Blob, OldBlob string
	}

	Edit struct {
		File    string
		Span    entities.Span
		Text    string
		Imports []string
		Keep    []entities.DeclKey
	}

	Verdict struct {
		IsBuilt     bool
		IsUnchecked bool
		IsUnfit     bool
		Detail      string
	}

	Conflict struct {
		Rev      string            `json:"rev"`
		Position entities.Position `json:"position"`
		Detail   string            `json:"detail"`
		Decls    []ConflictDecl    `json:"decls"`
	}

	ConflictDecl struct {
		Key      entities.DeclKey `json:"key"`
		Preimage string           `json:"-"`
		Final    string           `json:"-"`
	}

	StoppedError struct{ Conflicts []Conflict }
)

func keysByFile(graph entities.Graph) map[string][]entities.DeclKey {
	out := map[string][]entities.DeclKey{}

	for _, decl := range graph.Decls() {
		out[decl.Key.File] = append(out[decl.Key.File], decl.Key)
	}

	return out
}

func editsOf(active map[entities.DeclKey]entry, keep map[string][]entities.DeclKey) []Edit {
	edits := make([]Edit, 0, len(active))

	for _, placed := range active {
		edits = append(edits, Edit{
			File:    placed.key.File,
			Span:    placed.span,
			Text:    placed.preimage,
			Imports: placed.imports,
			Keep:    keep[placed.key.File],
		})
	}

	slices.SortFunc(edits, func(left, right Edit) int {
		return cmp.Or(
			strings.Compare(left.File, right.File), cmp.Compare(left.Span.Start, right.Span.Start),
			cmp.Compare(left.Span.End, right.Span.End), strings.Compare(left.Text, right.Text),
		)
	})

	return slices.CompactFunc(edits, func(left, right Edit) bool {
		return left.File == right.File && left.Span == right.Span && left.Text == right.Text
	})
}

func (err *StoppedError) Error() string {
	return fmt.Sprintf("%d anchors stopped for a decision", len(err.Conflicts))
}

func (err *StoppedError) Is(target error) bool { return target == ErrStopped }

func conflictOf(one found, detail string) Conflict {
	decls := make([]ConflictDecl, 0, len(one.entries))
	for _, placed := range one.entries {
		decls = append(decls, ConflictDecl{
			Key: placed.key, Preimage: placed.preimage, Final: placed.final,
		})
	}

	return Conflict{
		Rev: one.anchor.Rev, Position: one.anchor.Position, Detail: detail, Decls: decls,
	}
}

func (judge judgement) finish(chosen []kept, err error) ([]kept, error) {
	if err != nil || len(judge.stops) == 0 {
		return chosen, err
	}

	conflicts := slices.SortedFunc(maps.Values(judge.stops), func(left, right Conflict) int {
		return cmp.Compare(left.Position, right.Position)
	})

	return nil, &StoppedError{Conflicts: conflicts}
}

func (planner Planner) stop(one found, detail string, judge judgement) {
	judge.stops[one.anchor.Rev] = conflictOf(one, detail)

	planner.summarize(one, "stopped", detail)
}

//go:generate go tool -modfile=../../../../tools/go.mod mockgen -source=ports.go -destination=ports_mock_test.go -package=anchoring
