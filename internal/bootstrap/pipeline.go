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
	"github.com/wrapped-owls/redigit/internal/infra/langpack/dart"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/buildtags"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/web"
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
	session   closer
	reader    closer
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

type readerCloser struct{ reader gitstore.Reader }

func (adapter readerCloser) Close(context.Context) error {
	// The reader closes without a context; fitting it to the closer port lets
	// its cat-file process be released with the rest.
	if err := adapter.reader.Close(); err != nil {
		return fmt.Errorf("stop cat-file: %w", err)
	}

	return nil
}

var (
	_ closer  = (*gitstore.Checkout)(nil)
	_ closer  = (*lsp.Session)(nil)
	_ closer  = readerCloser{}
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

	opened, err := openRevision(ctx, root, cfg.Rev)
	if err != nil {
		return nil, err
	}

	reader, snapshot, checkout := opened.reader, opened.snapshot, opened.checkout
	gitReader := readerCloser{reader: reader}

	session, err := startSession(ctx, cfg, checkout)
	if err != nil {
		release(ctx, checkout, gitReader)

		return nil, err
	}

	indexCfg := cfg.Indexing
	configSyntax := config.NewSyntax(configChunkers(cfg.ChunkMap))
	specific := specificLanguages(
		configSyntax,
		finalTree{snapshot.Files, reader, cfg.Virtual},
		cfg.BuildTags,
	)

	if err = validateChunkMap(cfg.ChunkMap, snapshot, specific); err != nil {
		release(ctx, session, gitReader, checkout)

		return nil, fmt.Errorf("validate chunk map: %w", err)
	}

	indexCfg.Languages = append(specific, opaqueLanguage(configSyntax))

	// The snapshot describes a REVISION, so its root is where that revision
	// was materialised - not the repository, whose working tree holds whatever
	// happens to be checked out. A language pack that reads a sibling file
	// from disk (a generator directive, a mock header) would otherwise read
	// the wrong bytes, or fail on a file the revision has and the checkout
	// does not.
	snapshot.Root = checkout.Dir()

	source, err := newTreeSource(ctx, reader)
	if err != nil {
		release(ctx, session, gitReader, checkout)

		return nil, err
	}

	scanner := indexing.NewScanner(reader, indexCfg)

	if err = loadCapabilities(ctx, &cfg, scanner, snapshot); err != nil {
		release(ctx, session, gitReader, checkout)

		return nil, err
	}

	cfg.Auditing.Unseen, err = unseen(snapshot, cfg.BuildTags)
	if err != nil {
		release(ctx, session, gitReader, checkout)

		return nil, err
	}

	cfg.Auditing.Unclaimed = unclaimed(snapshot, indexCfg.Languages)

	return &Pipeline{
		checkout:  checkout,
		session:   session,
		reader:    gitReader,
		indexer:   indexing.New(session, reader, indexCfg),
		scanner:   scanner,
		orderer:   ordering.New(cfg.Ordering),
		planner:   planning.New(cfg.Planning),
		auditor:   auditing.New(cfg.Auditing),
		unseen:    cfg.Auditing.Unseen,
		unclaimed: cfg.Auditing.Unclaimed,
		snapshot:  snapshot,
		source:    source,
		renderer:  newRenderer(configSyntax, renderersByExtension(specific)),
	}, nil
}

type revision struct {
	reader   gitstore.Reader
	snapshot entities.Snapshot
	checkout *gitstore.Checkout
}

func openRevision(ctx context.Context, root, rev string) (revision, error) {
	reader := gitstore.NewReader(root, rev)

	snapshot, err := reader.Snapshot(ctx)
	if err != nil {
		release(ctx, readerCloser{reader: reader})

		return revision{}, fmt.Errorf("read snapshot: %w", err)
	}

	// The language server reads a working tree, so the revision has to exist
	// as one. Pointing it at the repository's own checkout would answer about
	// whatever is checked out there instead, and answer confidently.
	checkout, err := gitstore.NewCheckout(ctx, root, rev)
	if err != nil {
		release(ctx, readerCloser{reader: reader})

		return revision{}, fmt.Errorf("materialise revision: %w", err)
	}

	return revision{reader: reader, snapshot: snapshot, checkout: checkout}, nil
}

func startSession(
	ctx context.Context,
	cfg Config,
	checkout *gitstore.Checkout,
) (*lsp.Session, error) {
	session, err := lsp.StartSession(ctx, lsp.ServerConfig{
		Command:    cfg.ServerCommand,
		Args:       cfg.ServerArgs,
		RootPath:   checkout.Dir(),
		BuildFlags: buildFlags(cfg.BuildTags),
	})
	if err != nil {
		return nil, fmt.Errorf("start language server: %w", err)
	}

	return session, nil
}

// release closes what a failed start already opened. The start's own error is
// the one worth reporting, so a close error is dropped.
func release(ctx context.Context, opened ...closer) {
	for _, c := range opened {
		_ = c.Close(ctx)
	}
}

// unseen names the files this run's build tags leave invisible to the
// reference queries. It is computed here, at the composition root, because
// which constraints a tag satisfies is a per-language question and the
// auditing use case knows no language.
func unseen(snap entities.Snapshot, tags []string) ([]auditing.Unseen, error) {
	constrained, err := buildtags.BuildConstraints(snap)
	if err != nil {
		return nil, fmt.Errorf("read build constraints: %w", err)
	}

	hidden, err := buildtags.Hidden(constrained, tags)
	if err != nil {
		return nil, fmt.Errorf("evaluate build constraints: %w", err)
	}

	out := make([]auditing.Unseen, 0, len(hidden))
	for _, tag := range hidden {
		out = append(out, auditing.Unseen{
			File:       tag.File,
			Constraint: tag.Constraint,
			Tags:       tag.Tags,
		})
	}

	return out, nil
}

// unclaimed names every final-tree path no configured language's extension
// list matches. It is computed here, at the composition root, for the same
// reason unseen is: which extensions a language owns is a per-language
// question, and the auditing use case knows no language.
//
// This is the loud version of the indexer's own read loop
// (internal/domain/usecases/indexing/indexer.go), which skips exactly these
// paths with a bare continue and reports nothing - a plan built from this
// run can never reproduce one of them, and without this, nothing says so
// until the final-tree invariant fails at the very end.
func unclaimed(snap entities.Snapshot, langs []indexing.Language) []auditing.Unclaimed {
	out := make([]auditing.Unclaimed, 0, len(snap.Files))

	for _, file := range snap.Files {
		if !slices.ContainsFunc(
			langs,
			func(lang indexing.Language) bool { return lang.Owns(file) },
		) {
			out = append(out, auditing.Unclaimed{File: file})
		}
	}

	return out
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
// Close stops the language server and the git reader, then removes the
// throwaway checkout, folding every failure so one does not hide another.
func (p *Pipeline) Close(ctx context.Context) error {
	var closeErr error

	if err := p.session.Close(ctx); err != nil {
		closeErr = errors.Join(closeErr, fmt.Errorf("close language server: %w", err))
	}

	if err := p.reader.Close(ctx); err != nil {
		closeErr = errors.Join(closeErr, fmt.Errorf("close git reader: %w", err))
	}

	if err := p.checkout.Close(ctx); err != nil {
		closeErr = errors.Join(closeErr, fmt.Errorf("remove checkout: %w", err))
	}

	return closeErr
}

// finalTree is what the Dart and web packs resolve an import against: which
// files the tree holds, how to read one, and where each virtual module a
// bundler plugin makes up is compiled from.
type finalTree struct {
	files   []string
	reader  treeReader
	virtual map[string]string
}

type treeReader interface {
	Read(ctx context.Context, path string) ([]byte, error)
}

// specificLanguages is what the indexer reads a known format with. The config
// entry covers the files a build needs and no language server reads: without
// them a plan holds no go.mod, so every tree it describes has source and
// nothing to compile it.
//
// Kept apart from opaqueLanguage so validateChunkMap can check a supplied
// chunk map against real ownership before the catch-all pack - which would
// otherwise also claim every path - is added.
//
// Dart and the web files are not Resolved: the one language server a run
// starts is gopls, so their rule packs carry the reference edges themselves.
func specificLanguages(
	configSyntax config.Syntax,
	tree finalTree,
	tags []string,
) []indexing.Language {
	return []indexing.Language{
		{
			Name:       "go",
			Extensions: []string{".go"},
			Syntax:     golang.NewSyntax(),
			Rules:      golang.NewRulePack(tags),
			Resolved:   true,
		},
		{
			Name:       "dart",
			Extensions: []string{".dart"},
			Syntax:     dart.NewSyntax(tree.files, tree.reader),
			Rules:      dart.NewRulePack(),
		},
		{
			Name:       "web",
			Extensions: web.Extensions(),
			Syntax:     web.NewSyntax(tree.files, tree.reader),
			Rules:      web.NewRulePack(tree.virtual),
		},
		{
			Name:       "config",
			Extensions: configExtensions(),
			Syntax:     configSyntax,
			Rules:      config.NewRulePack(),
		},
	}
}

// opaqueLanguage claims every final-tree path neither "go" nor "config" owns.
// It shares configSyntax's own chunker list, so it reads such a path
// whole-file by default (chunk.WholeFile) unless a chunk map entry refines
// the cut - see docs/PLANO-EMISSAO.md:113-133. Without it, a format this tool
// has no reader for - .tmpl, a golden fixture - vanishes from the graph
// in silence and a plan built from this run can never reproduce it.
func opaqueLanguage(configSyntax config.Syntax) indexing.Language {
	return indexing.Language{
		Name:       "opaque",
		Extensions: []string{indexing.AnyExtension},
		Syntax:     configSyntax,
	}
}
