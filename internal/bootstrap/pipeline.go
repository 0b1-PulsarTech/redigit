package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/auditing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/emitting"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/ordering"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/config"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
	"github.com/wrapped-owls/redigit/internal/infra/lsp"
)

// Pipeline holds the stages wired for one run, plus the language server that
// answers for them. Close stops that server and the git reader's processes.
//
// Each stage is held through a small local interface, not the concrete
// domain type, so a test can drive Pipeline's own error-wrapping - Graph,
// Order, Plan, Audit, Births, Close - with a fake stage instead of a real
// language server, git worktree, or parsed source tree.
type Pipeline struct {
	checkout  closer
	loads     closer
	session   closer
	reader    closer
	hooked    closers
	indexer   indexer
	scanner   scanner
	orderer   orderer
	planner   planner
	auditor   auditor
	snapshot  entities.Snapshot
	source    emitting.Source
	renderer  emitting.Renderer
	unseen    []auditing.Unseen
	unclaimed []auditing.Unclaimed
}

// Unseen names the files a build constraint hid from this run's reference
// queries. A caller reports it so a graph built without the tags is not read
// as a graph built with them.
func (p *Pipeline) Unseen() []auditing.Unseen { return p.unseen }

// Unclaimed names the final-tree paths no configured language pack claims. A
// caller reports it because a plan built from this run can never reproduce
// one of these paths at all, which is a different failure than Unseen: that
// one still answers the syntactic query, this one is never read.
func (p *Pipeline) Unclaimed() []auditing.Unclaimed { return p.unclaimed }

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
	_ closer  = (*typed.Source)(nil)
	_ closer  = readerCloser{}
	_ indexer = indexing.Indexer{}
	_ scanner = indexing.Scanner{}
	_ orderer = ordering.Orderer{}
	_ planner = planning.Planner{}
	_ auditor = auditing.Auditor{}
)

// Start builds the pipeline: it reads the snapshot, starts the language server (at once for lsp,
// on the first fallback for typed), and wires each stage to the ports it consumes. This is the
// only place that names a concrete adapter.
func Start(ctx context.Context, cfg Config) (*Pipeline, error) {
	return start(ctx, cfg, stageHooks())
}

func start(ctx context.Context, cfg Config, hooks []namedHook) (*Pipeline, error) {
	root, err := filepath.Abs(cfg.Repo)
	if err != nil {
		return nil, fmt.Errorf("resolve repository path: %w", err)
	}

	opened, err := openRevision(ctx, root, cfg)
	if err != nil {
		return nil, err
	}

	reader, snapshot, checkout := opened.reader, opened.snapshot, opened.checkout
	gitReader := readerCloser{reader: reader}
	goSource := typed.NewSource(cfg.BuildTags)

	wired, err := wireResolver(ctx, cfg, snapshot, goSource, serverStarter(cfg, checkout.Dir()))
	if err != nil {
		release(ctx, goSource, checkout, gitReader)

		return nil, err
	}

	configSyntax := config.NewSyntax(configChunkers(cfg.ChunkMap))
	tree := finalTree{snapshot.Files, reader, cfg.Virtual}
	specific := specificLanguages(configSyntax, tree, goSource)

	env := stageEnv{root: root, opened: opened, specific: specific, goSource: goSource, tree: tree}

	indexCfg, built, err := shapeStages(ctx, &cfg, env, configSyntax, hooks)
	if err != nil {
		release(ctx, goSource, wired.session, gitReader, checkout)

		return nil, err
	}

	// A hook's resources read the session and the checkout, so they close before them.
	fail := func(err error) error {
		release(ctx, built.closers, goSource, wired.session, gitReader, checkout)

		return err
	}

	source, err := newTreeSource(ctx, reader)
	if err != nil {
		return nil, fail(err)
	}

	scanner := indexing.NewScanner(reader, indexCfg)

	if err = loadCapabilities(ctx, &cfg, scanner, snapshot); err != nil {
		return nil, fail(err)
	}

	cfg.Auditing.Unseen, err = unseen(snapshot, cfg.BuildTags)
	if err != nil {
		return nil, fail(err)
	}

	cfg.Auditing.Unclaimed = unclaimed(snapshot, indexCfg.Languages)

	return &Pipeline{
		checkout:  checkout,
		loads:     goSource,
		session:   wired.session,
		reader:    gitReader,
		hooked:    built.closers,
		indexer:   wired.guard(indexing.New(wired.resolver, reader, indexCfg)),
		scanner:   scanner,
		orderer:   built.orderer,
		planner:   built.planner,
		auditor:   auditing.New(cfg.Auditing),
		unseen:    cfg.Auditing.Unseen,
		unclaimed: cfg.Auditing.Unclaimed,
		snapshot:  snapshot,
		source:    source,
		renderer:  newRenderer(configSyntax, renderersByExtension(specific)),
	}, nil
}

type (
	stageHook func(context.Context, *Config, stageEnv, *stages) error

	namedHook struct {
		name string
		run  stageHook
	}

	stageEnv struct {
		root     string
		opened   revision
		specific []indexing.Language
		goSource *typed.Source
		tree     finalTree
	}

	stages struct {
		languages []indexing.Language
		orderer   orderer
		planner   planner
		closers   closers
	}
)

func shapeStages(
	ctx context.Context,
	cfg *Config,
	env stageEnv,
	configSyntax config.Syntax,
	hooks []namedHook,
) (indexing.Config, *stages, error) {
	if err := validateChunkMap(cfg.ChunkMap, env.opened.snapshot, env.specific); err != nil {
		return indexing.Config{}, nil, fmt.Errorf("validate chunk map: %w", err)
	}

	built := &stages{orderer: ordering.New(cfg.Ordering), planner: planning.New(cfg.Planning)}

	if err := runHooks(ctx, cfg, env, built, hooks); err != nil {
		return indexing.Config{}, nil, err
	}

	indexCfg := cfg.Indexing
	indexCfg.Languages = slices.Concat(
		env.specific, built.languages, []indexing.Language{opaqueLanguage(configSyntax)},
	)

	return indexCfg, built, nil
}

func runHooks(
	ctx context.Context,
	cfg *Config,
	env stageEnv,
	built *stages,
	hooks []namedHook,
) error {
	for _, hook := range hooks {
		if err := hook.run(ctx, cfg, env, built); err != nil {
			release(ctx, built.closers)

			return fmt.Errorf("%s: %w", hook.name, err)
		}
	}

	return nil
}

type revision struct {
	reader   gitstore.Reader
	snapshot entities.Snapshot
	checkout *gitstore.Checkout
}

func openRevision(ctx context.Context, root string, cfg Config) (revision, error) {
	reader := gitstore.NewReader(root, cfg.Rev)

	snapshot, err := reader.Snapshot(ctx)
	if err != nil {
		release(ctx, readerCloser{reader: reader})

		return revision{}, fmt.Errorf("read snapshot: %w", err)
	}

	// Before the checkout and the language server: a rule naming nothing in
	// the tree is a config mistake, and starting gopls costs about 40 s.
	if err = validateRules(cfg, snapshot); err != nil {
		release(ctx, readerCloser{reader: reader})

		return revision{}, err
	}

	// The language server reads a working tree, so the revision has to exist
	// as one. Pointing it at the repository's own checkout would answer about
	// whatever is checked out there instead, and answer confidently.
	checkout, err := gitstore.NewCheckout(ctx, root, cfg.Rev)
	if err != nil {
		release(ctx, readerCloser{reader: reader})

		return revision{}, fmt.Errorf("materialise revision: %w", err)
	}

	// The snapshot describes a REVISION, so its root is where that revision was materialised, not
	// the repository whose working tree holds whatever happens to be checked out. A language pack
	// that reads a sibling file from disk (a generator directive, a mock header) would otherwise
	// read the wrong bytes, or fail on a file the revision has and the checkout does not.
	snapshot.Root = checkout.Dir()

	return revision{reader: reader, snapshot: snapshot, checkout: checkout}, nil
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

// Close stops what the hooks opened, the language server, the typed loads and the git reader, then
// removes the throwaway checkout, folding every failure so one does not hide
// another; the checkout is kept when the session or the loads fail to stop.
// It is registered by the caller right after Start, so a failure later still
// releases the process.
func (p *Pipeline) Close(ctx context.Context) error {
	// The session and typed loads read the checkout and stop first; one that fails keeps it.
	// The resolver goes before them: its index build reads the loads, so closing them under it
	// would log a skipped ad hoc pass that was only cancelled.
	hooksErr := p.hooked.Close(ctx)
	sessionErr := p.session.Close(ctx)
	loadsErr := p.loads.Close(ctx)
	closeErr := errors.Join(
		stopped("close hook resources", hooksErr),
		stopped("close language server", sessionErr),
		stopped("stop typed loads", loadsErr),
		stopped("close git reader", p.reader.Close(ctx)),
	)

	if keep := errors.Join(sessionErr, loadsErr); keep != nil {
		return errors.Join(closeErr, fmt.Errorf("keep checkout %s: %w", p.snapshot.Root, keep))
	}

	return errors.Join(closeErr, stopped("remove checkout", p.checkout.Close(ctx)))
}

func stopped(name string, err error) error {
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}

	return nil
}
