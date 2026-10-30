package profiling

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"runtime/trace"
	"slices"
)

type Options struct {
	CPUProfile string
	MemProfile string
	Trace      string
	MemLimit   string
}

type Session struct {
	stops []func() error
}

var ErrSamePath = errors.New("profile outputs must be different files")

func Start(opts Options) (*Session, error) {
	if err := distinct(opts); err != nil {
		return nil, err
	}

	session := &Session{}

	if opts.MemLimit != "" {
		limit, err := ParseMemLimit(opts.MemLimit)
		if err != nil {
			return nil, err
		}

		previous := debug.SetMemoryLimit(limit)

		session.stops = append(session.stops, func() error {
			debug.SetMemoryLimit(previous)

			return nil
		})
	}

	if opts.CPUProfile != "" {
		if err := session.startCPU(opts.CPUProfile); err != nil {
			return nil, errors.Join(err, session.Stop())
		}
	}

	if opts.Trace != "" {
		if err := session.startTrace(opts.Trace); err != nil {
			return nil, errors.Join(err, session.Stop())
		}
	}

	if opts.MemProfile != "" {
		if err := session.startHeap(opts.MemProfile); err != nil {
			return nil, errors.Join(err, session.Stop())
		}
	}

	return session, nil
}

func (session *Session) Stop() error {
	// Run once: a second call finds nothing left to stop.
	if session == nil {
		return nil
	}

	stops := session.stops
	session.stops = nil

	var stopErr error

	for _, stop := range slices.Backward(stops) {
		stopErr = errors.Join(stopErr, stop())
	}

	return stopErr
}

func (session *Session) startCPU(path string) error {
	file, err := createFile(path)
	if err != nil {
		return err
	}

	if err = pprof.StartCPUProfile(file); err != nil {
		return errors.Join(fmt.Errorf("start cpu profile: %w", err), file.Close())
	}

	session.stops = append(session.stops, func() error {
		pprof.StopCPUProfile()

		return closeFile(file)
	})

	return nil
}

func (session *Session) startTrace(path string) error {
	file, err := createFile(path)
	if err != nil {
		return err
	}

	if err = trace.Start(file); err != nil {
		return errors.Join(fmt.Errorf("start trace: %w", err), file.Close())
	}

	session.stops = append(session.stops, func() error {
		trace.Stop()

		return closeFile(file)
	})

	return nil
}

func (session *Session) startHeap(path string) error {
	file, err := createFile(path)
	if err != nil {
		return err
	}

	session.stops = append(session.stops, func() error { return writeHeap(file) })

	return nil
}

func writeHeap(file *os.File) error {
	// A collection first, so the profile shows what is live and not garbage.
	runtime.GC()

	writeErr := pprof.Lookup("heap").WriteTo(file, 0)
	if writeErr != nil {
		writeErr = fmt.Errorf("write heap profile: %w", writeErr)
	}

	return errors.Join(writeErr, closeFile(file))
}
