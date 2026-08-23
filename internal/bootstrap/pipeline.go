package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/auditing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/emitting"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/ordering"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/config"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang"
	"github.com/wrapped-owls/redigit/internal/infra/lsp"
)

// Pipeline holds the stages wired for one run, plus the language server that
// answers for them. Close stops that server; nothing else here owns a process.
//
// Each stage is held through a small local interface, not the concrete
// domain type, so a test can drive Pipeline's own error-wrapping - Graph,
// Order, Plan, Audit, Births, Close - with a fake stage instead of a real
// language server, git worktree, or parsed source tree.
type Pipeline struct {
	checkout closer
	session  closer
	indexer  indexer
	scanner  scanner
	orderer  orderer
	planner  planner
	auditor  auditor
	snapshot entities.Snapshot
	source   emitting.Source
	renderer emitting.Renderer
}

// closer is what Pipeline.Close needs from the language server session and
// the checkout it made.
type closer interface {
	Close(ctx context.Context) error
}

type indexer interface {
	Index(ctx context.Context, snap entities.Snapshot) (indexing.Result, error)
}

type scanner interface {
	Scan(ctx context.Context, snap entities.Snapshot) (indexing.Births, error)
}

type orderer interface {
	Order(graph entities.Graph, snap entities.Snapshot) (entities.Ordering, error)
}

type planner interface {
	Plan(graph entities.Graph, order entities.Ordering) (entities.Plan, error)
}

type auditor interface {
	Audit(graph entities.Graph, order entities.Ordering) (auditing.Result, error)
}

var (
	_ closer  = (*gitstore.Checkout)(nil)
	_ closer  = (*lsp.Session)(nil)
	_ indexer = indexing.Indexer{}
	_ scanner = indexing.Scanner{}
	_ orderer = ordering.Orderer{}
	_ planner = planning.Planner{}
	_ auditor = auditing.Auditor{}
)

// Start builds the pipeline: it reads the snapshot, launches the language
// server, and wires each stage to the ports it consumes. This is the only
// place that names a concrete adapter.
func Start(ctx context.Context, cfg Config) (*Pipeline, error) {
	root, err := filepath.Abs(cfg.Repo)
	if err != nil {
		return nil, fmt.Errorf("resolve repository path: %w", err)
	}

	reader := gitstore.NewReader(root, cfg.Rev)

	snapshot, err := reader.Snapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("read snapshot: %w", err)
	}

	// The language server reads a working tree, so the revision has to exist
	// as one. Pointing it at the repository's own checkout would answer about
	// whatever is checked out there instead, and answer confidently.
	checkout, err := gitstore.NewCheckout(ctx, root, cfg.Rev)
	if err != nil {
		return nil, fmt.Errorf("materialise revision: %w", err)
	}

	session, err := lsp.StartSession(ctx, lsp.ServerConfig{
		Command:    cfg.ServerCommand,
		Args:       cfg.ServerArgs,
		RootPath:   checkout.Dir(),
		BuildFlags: buildFlags(cfg.BuildTags),
	})
	if err != nil {
		_ = checkout.Close(ctx)

		return nil, fmt.Errorf("start language server: %w", err)
	}

	indexCfg := cfg.Indexing
	configSyntax := config.NewSyntax(configChunkers())
	indexCfg.Languages = languages(configSyntax)

	// The snapshot describes a REVISION, so its root is where that revision
	// was materialised - not the repository, whose working tree holds whatever
	// happens to be checked out. A language pack that reads a sibling file
	// from disk (a generator directive, a mock header) would otherwise read
	// the wrong bytes, or fail on a file the revision has and the checkout
	// does not.
	snapshot.Root = checkout.Dir()

	source, err := newTreeSource(ctx, reader)
	if err != nil {
		_ = session.Close(ctx)
		_ = checkout.Close(ctx)

		return nil, err
	}

	scanner := indexing.NewScanner(reader, indexCfg)

	if err = loadCapabilities(ctx, &cfg, scanner, snapshot); err != nil {
		_ = session.Close(ctx)
		_ = checkout.Close(ctx)

		return nil, err
	}

	return &Pipeline{
		checkout: checkout,
		session:  session,
		indexer:  indexing.New(session, reader, indexCfg),
		scanner:  scanner,
		orderer:  ordering.New(cfg.Ordering),
		planner:  planning.New(cfg.Planning),
		auditor:  auditing.New(cfg.Auditing),
		snapshot: snapshot,
		source:   source,
		renderer: newRenderer(golang.NewSyntax(), configRenderers(configSyntax)),
	}, nil
}

func buildFlags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}

	flags := make([]string, 0, len(tags))
	for _, tag := range tags {
		flags = append(flags, "-tags="+tag)
	}

	return flags
}

func (p *Pipeline) Snapshot() entities.Snapshot { return p.snapshot }

// Births dates each declaration by the first commit whose version of its file
// already held it. That is the answer the file's own first touch only
// approximates, and the approximation is what dates a late-grown declaration
// to the moment its file was created nearly empty.
func (p *Pipeline) Births(ctx context.Context) (indexing.Births, error) {
	births, err := p.scanner.Scan(ctx, p.snapshot)
	if err != nil {
		return indexing.Births{}, fmt.Errorf("scan births: %w", err)
	}

	return births, nil
}

// Graph indexes the snapshot.
func (p *Pipeline) Graph(ctx context.Context) (indexing.Result, error) {
	result, err := p.indexer.Index(ctx, p.snapshot)
	if err != nil {
		return result, fmt.Errorf("index: %w", err)
	}

	return result, nil
}

// Order settles the graph. The ordering is returned even alongside an error:
// a cycle or an unresolved declaration is data a caller reports on, not merely
// a reason to hand back nothing.
func (p *Pipeline) Order(graph entities.Graph) (entities.Ordering, error) {
	order, err := p.orderer.Order(graph, p.snapshot)
	if err != nil {
		return order, fmt.Errorf("order: %w", err)
	}

	return order, nil
}

func (p *Pipeline) Plan(graph entities.Graph, order entities.Ordering) (entities.Plan, error) {
	plan, err := p.planner.Plan(graph, order)
	if err != nil {
		return entities.Plan{}, fmt.Errorf("plan: %w", err)
	}

	return plan, nil
}

func (p *Pipeline) Audit(graph entities.Graph, order entities.Ordering) (auditing.Result, error) {
	result, err := p.auditor.Audit(graph, order)
	if err != nil {
		return auditing.Result{}, fmt.Errorf("audit: %w", err)
	}

	return result, nil
}

// Close stops the language server. It is registered by the caller right after
// Start, so a failure later still releases the process.
// Close stops the language server and removes the throwaway checkout, in that
// order, folding both failures so one does not hide the other.
func (p *Pipeline) Close(ctx context.Context) error {
	var closeErr error

	if err := p.session.Close(ctx); err != nil {
		closeErr = errors.Join(closeErr, fmt.Errorf("close language server: %w", err))
	}

	if err := p.checkout.Close(ctx); err != nil {
		closeErr = errors.Join(closeErr, fmt.Errorf("remove checkout: %w", err))
	}

	return closeErr
}

// languages is what the indexer reads with. The config entry covers the files
// a build needs and no language server reads: without them a plan holds no
// go.mod, so every tree it describes has source and nothing to compile it.
func languages(configSyntax config.Syntax) []indexing.Language {
	return []indexing.Language{
		{
			Name:       "go",
			Extensions: []string{".go"},
			Syntax:     golang.NewSyntax(),
			Rules:      golang.NewRulePack(),
			Resolved:   true,
		},
		{
			Name:       "config",
			Extensions: configExtensions(),
			Syntax:     configSyntax,
			Rules:      config.NewRulePack(),
		},
	}
}
