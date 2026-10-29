package batch

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

const (
	pipeBuffer = 64 << 10
	MaxObject  = 256 << 20
)

var (
	ErrMissing   = errors.New("object missing")
	ErrClosed    = errors.New("batch stream closed")
	ErrProcess   = errors.New("batch process failed")
	ErrBadSpec   = errors.New("invalid object spec")
	ErrAmbiguous = errors.New("ambiguous object name")
	ErrNotBlob   = errors.New("object is not a blob")
	ErrTooLarge  = errors.New("object too large")
	errProtocol  = errors.New("unexpected cat-file reply")
	errUndrained = errors.New("object too large to drain")
)

type Stream struct {
	repo string

	reads sync.Mutex
	state sync.Mutex
	proc  *process
	shut  bool
}

func NewStream(repo string) *Stream { return &Stream{repo: repo} }

func (stream *Stream) Read(ctx context.Context, spec string) ([]byte, error) {
	if strings.ContainsAny(spec, "\n\x00") || spec == "" {
		return nil, fmt.Errorf("%w: spec holds a line break or is empty", ErrBadSpec)
	}

	stream.reads.Lock()
	defer stream.reads.Unlock()

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("read object: %w", err)
	}

	proc, err := stream.current(ctx)
	if err != nil {
		return nil, err
	}

	// Killing the child is the only way to unblock a pipe read on cancel.
	stop := context.AfterFunc(ctx, proc.kill)

	content, err := proc.exchange(spec)
	isLive := stop()

	if !isLive || (err != nil && (!isVerdict(err) || errors.Is(err, errUndrained))) {
		stream.drop(proc)
	}

	if err != nil {
		if isVerdict(err) {
			return nil, err
		}

		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("read object: %w", ctxErr)
		}

		detail := proc.stderr.String()

		return nil, fmt.Errorf("%w: %w: %s", ErrProcess, err, detail)
	}

	return content, nil
}

func (stream *Stream) Close() error {
	stream.state.Lock()
	stream.shut = true
	proc := stream.proc
	stream.state.Unlock()

	if proc == nil {
		return nil
	}

	proc.kill()
	stream.reads.Lock()
	stream.drop(proc)
	stream.reads.Unlock()

	return nil
}

func (stream *Stream) current(ctx context.Context) (*process, error) {
	stream.state.Lock()
	defer stream.state.Unlock()

	if stream.shut {
		return nil, ErrClosed
	}

	if stream.proc != nil {
		return stream.proc, nil
	}

	proc, err := start(ctx, stream.repo)
	if err != nil {
		return nil, err
	}

	stream.proc = proc

	return proc, nil
}

func (stream *Stream) drop(proc *process) {
	stream.state.Lock()
	if stream.proc == proc {
		stream.proc = nil
	}
	stream.state.Unlock()

	proc.retire()
}

func isVerdict(err error) bool {
	return errors.Is(err, ErrMissing) || errors.Is(err, ErrAmbiguous) ||
		errors.Is(err, ErrNotBlob) || errors.Is(err, ErrTooLarge)
}
