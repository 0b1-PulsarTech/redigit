package rewriting

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// Config names the refs the rewrite reads and writes.
type Config struct {
	// Out is the ref the rebuilt history lands on.
	Out string
	// Target is the ref holding the tree the rebuild must reproduce exactly.
	Target string
	// AllowDraftSubjects lets a rebuild run on a plan nobody reviewed. It
	// exists so a dry run can happen at all, and defaults off because a
	// generated subject describes what a commit declares, not what it does.
	AllowDraftSubjects bool
}

// Rewriter turns a reviewed plan into a real history. It renders each file from
// its FINAL text holding only the declarations born so far, so a file grows
// across commits instead of appearing whole.
type Rewriter struct {
	source   Source
	renderer Renderer
	history  History
	cfg      Config
}

func New(source Source, renderer Renderer, history History, cfg Config) Rewriter {
	return Rewriter{source: source, renderer: renderer, history: history, cfg: cfg}
}

// Rewrite writes the plan and proves the final tree matches the target.
func (r Rewriter) Rewrite(ctx context.Context, plan entities.Plan) (string, error) {
	if err := r.check(plan); err != nil {
		return "", err
	}

	commits, err := r.build(ctx, plan)
	if err != nil {
		return "", err
	}

	ref, err := r.history.Write(ctx, r.cfg.Out, commits)
	if err != nil {
		return "", fmt.Errorf("write history: %w", err)
	}

	// The invariant is the net, not the verdict. It proves nothing was lost;
	// it says nothing about whether the middle of the history makes sense,
	// which is a separate measurement the auditor reports on its own.
	if err = r.history.Verify(ctx, ref, r.cfg.Target); err != nil {
		return ref, fmt.Errorf("verify rebuild: %w", err)
	}

	return ref, nil
}

func (r Rewriter) check(plan entities.Plan) error {
	if plan.Len() == 0 {
		return fmt.Errorf("%w: the plan holds no commit", apperr.ErrIncomplete)
	}

	if len(plan.Unplaced) > 0 {
		return fmt.Errorf(
			"%w: %d declarations are in no commit, starting at %s",
			apperr.ErrIncomplete, len(plan.Unplaced), plan.Unplaced[0],
		)
	}

	if r.cfg.AllowDraftSubjects {
		return nil
	}

	for _, commit := range plan.Commits {
		if entities.IsDraft(commit.Subject) {
			return fmt.Errorf(
				"%w: commit %d still carries a generated subject: %q",
				apperr.ErrIncomplete, commit.Position, commit.Subject,
			)
		}
	}

	return nil
}

// build renders one commit per planned commit. A file is re-rendered whenever
// the set of its declarations that have been born changes, and only then, so a
// commit's diff holds exactly the declarations that commit introduces.
func (r Rewriter) build(ctx context.Context, plan entities.Plan) ([]Commit, error) {
	born := map[string][]entities.DeclKey{}
	sources := map[string][]byte{}
	commits := make([]Commit, 0, plan.Len())

	for _, planned := range plan.Commits {
		touched := map[string]bool{}

		for _, key := range planned.Decls {
			born[key.File] = append(born[key.File], key)
			touched[key.File] = true
		}

		writes, err := r.renderTouched(ctx, touched, born, sources)
		if err != nil {
			return nil, err
		}

		commit := Commit{Subject: planned.Subject, Writes: writes}
		if commit.IsEmpty() {
			return nil, fmt.Errorf(
				"%w: commit %d (%q) changes nothing",
				apperr.ErrIncomplete, planned.Position, planned.Subject,
			)
		}

		commits = append(commits, commit)
	}

	return commits, nil
}

func (r Rewriter) renderTouched(
	ctx context.Context,
	touched map[string]bool,
	born map[string][]entities.DeclKey,
	sources map[string][]byte,
) ([]FileContent, error) {
	paths := make([]string, 0, len(touched))
	for path := range touched {
		paths = append(paths, path)
	}

	slices.SortFunc(paths, cmp.Compare)

	writes := make([]FileContent, 0, len(paths))

	for _, path := range paths {
		source, cached := sources[path]
		if !cached {
			read, err := r.source.Read(ctx, path)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", path, err)
			}

			source = read
			sources[path] = read
		}

		rendered, err := r.renderer.Render(ctx, source, born[path])
		if err != nil {
			return nil, fmt.Errorf("render %s: %w", path, err)
		}

		// A file whose every declaration is still unborn renders as nothing.
		// Writing it anyway would create a file holding only a package clause
		// and imports, which is the defect the previous rebuild shipped.
		if len(rendered) == 0 {
			continue
		}

		writes = append(writes, FileContent{Path: path, Content: rendered})
	}

	return writes, nil
}
