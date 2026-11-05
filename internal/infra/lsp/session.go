package lsp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

// defaultPositionEncoding is what this client assumes when a server's
// initialize response leaves capabilities.positionEncoding unset, which the
// LSP spec requires reading as UTF-16 and which gopls always does regardless
// of what general.positionEncodings this client advertised.
const defaultPositionEncoding = "utf-16"

// closeTimeout bounds how long Session.Close may take even when the caller
// passes a ctx with no deadline of its own (context.Background() is a legal
// value; nothing in this package's exported API forbids it). Close exists
// specifically to guarantee the session's resources stop, so it cannot
// simply inherit an undeadlined ctx and risk blocking forever behind a
// wedged shutdown call (see sendRacingCtx). This value is generous enough
// for a real language server's graceful shutdown/exit round trip - which
// normally completes in a small fraction of a second, even for a large
// workspace, since it is just two small frames over an already-open pipe -
// while still keeping Close's worst case bounded to a small, human-scale
// ceiling instead of an indefinite hang.
const closeTimeout = 5 * time.Second

// ErrEmptyCommand reports a ServerConfig with no server command to launch.
var ErrEmptyCommand = errors.New("lsp: server command is empty")

// ErrNoDefinition reports that a definition request found nothing, which
// Session.Definition returns instead of a zero-value entities.Location a
// caller could mistake for a real one.
var ErrNoDefinition = errors.New("lsp: no definition found")

// Session starts one language server process and answers indexing.Resolver
// against it. It owns the process and the single reader goroutine that
// drains its stdout; Close stops both.
//
// See client's doc comment for a residual limitation Session inherits: a
// call or notify whose own ctx has already fired can still be stuck behind
// an unrelated wedged write sharing the same connection, until Close (or
// the process context otherwise ending) tears the whole connection down.
type Session struct {
	client *client
	cancel context.CancelFunc
	group  *errgroup.Group
	// wait blocks until the underlying transport is fully gone (the server
	// process has exited); kill is a best-effort forceful stop used only
	// when initialize fails. Both are seams: StartSession wires them to
	// cmd.Wait/cmd.Process.Kill, tests wire them to a fake transport.
	wait func() error
	kill func() error

	// root is the workspace directory. Paths cross this package's boundary
	// repo-relative, the way the rest of the tool names a file, and are
	// absolute inside it, the way the protocol requires. Joining and
	// un-joining here is the adapter absorbing the difference; a caller that
	// had to remember which side it was on would eventually get it wrong.
	root string

	positionEncoding string
}

var _ indexing.Resolver = (*Session)(nil)

// newSession wires a client onto the given stdio transport and starts its
// reader goroutine under processCtx. The caller owns processCtx/cancel:
// StartSession derives them from a detached context (see above); cancel is
// what Close calls to stop the reader loop, independently of the process
// itself (see the exec.Command note in StartSession).
func newSession(
	processCtx context.Context,
	cancel context.CancelFunc,
	stdin io.Writer,
	stdout io.ReadCloser,
	logger *slog.Logger,
	wait, kill func() error,
) *Session {
	rpcClient := newClient(stdin, stdout, logger)
	group, gctx := errgroup.WithContext(processCtx)
	group.Go(func() error { return rpcClient.run(gctx) })

	return &Session{
		client:           rpcClient,
		cancel:           cancel,
		group:            group,
		wait:             wait,
		kill:             kill,
		positionEncoding: defaultPositionEncoding,
	}
}

// PositionEncoding reports the code-unit encoding negotiated with the
// server for every Position.Character this Session sends or reads, so a
// caller building a Position from a byte offset knows which conversion in
// this package to apply. It defaults to "utf-16" per spec, which is also
// gopls's actual behavior regardless of what was negotiated.
func (s *Session) PositionEncoding() string {
	return s.positionEncoding
}

// Close runs the shutdown/exit sequence and waits for the server process to
// exit, stopping the reader goroutine this Session owns; ctx past its
// deadline force-kills instead of blocking forever. Close's whole purpose
// is guaranteeing the session's resources stop, so it never simply trusts
// whatever ctx the caller passed: closeTimeout caps the total regardless of
// whether ctx carries its own deadline at all (context.Background() is
// legal here). context.WithTimeout composes correctly with a
// caller-supplied shorter deadline - the earlier of the two always wins -
// so this never weakens a caller's own tighter bound, it only adds a
// ceiling for the case where the caller passed none. A best-effort
// $/cancelRequest can still add up to cancelNotifyTimeout beyond that in
// edge cases (see waitBackground).
func (s *Session) Close(ctx context.Context) error {
	ctx, releaseBound := context.WithTimeout(ctx, closeTimeout)
	defer releaseBound()

	var errs []error

	if err := s.client.call(ctx, "shutdown", nil, nil); err != nil {
		errs = append(errs, fmt.Errorf("shutdown: %w", err))
	}

	if err := s.client.notify(ctx, "exit", nil); err != nil {
		errs = append(errs, fmt.Errorf("exit: %w", err))
	}

	// s.cancel stops the reader and, via run's watcher goroutine,
	// force-closes the writer once its ctx ends - this is what unblocks any
	// write still wedged on writeMu. waitBackground runs right after so a
	// detached cancel-notify goroutine (see client.call) errors out and
	// returns promptly against the now-torn-down transport instead of
	// running its own notify out to cancelNotifyTimeout; it runs before
	// s.group.Wait so a background write queued behind the reader's own
	// writeJSON contention cannot create ordering surprises with the
	// reader's own shutdown.
	s.cancel()
	s.client.waitBackground()

	if err := s.group.Wait(); err != nil && !readerEndedCleanly(err) {
		errs = append(errs, fmt.Errorf("reader: %w", err))
	}

	if err := s.waitOrKill(ctx); err != nil {
		errs = append(errs, fmt.Errorf("wait: %w", err))
	}

	return errors.Join(errs...)
}

// waitOrKill waits for the server process to exit, racing s.wait() against
// ctx in its own goroutine so it can return the instant ctx fires without
// leaking. If ctx wins, it force-kills via s.kill, then drains waitDone
// (kill makes cmd.Wait return) so that goroutine still gets a stop
// condition instead of being abandoned.
func (s *Session) waitOrKill(ctx context.Context) error {
	waitDone := make(chan error, 1)
	go func() { waitDone <- s.wait() }()

	select {
	case err := <-waitDone:
		return cleanExit(err)
	case <-ctx.Done():
		_ = s.kill()

		return errors.Join(ctx.Err(), <-waitDone)
	}
}
