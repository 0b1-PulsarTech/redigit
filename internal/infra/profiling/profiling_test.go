package profiling

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime/debug"
	"runtime/pprof"
	"runtime/trace"
	"testing"
)

func readFile(t *testing.T, path string) []byte {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return content
}

func TestStartWritesEveryProfile(t *testing.T) {
	dir := t.TempDir()
	opts := Options{
		CPUProfile: filepath.Join(dir, "cpu.pprof"),
		MemProfile: filepath.Join(dir, "mem.pprof"),
		Trace:      filepath.Join(dir, "run.trace"),
	}

	session, err := Start(opts)
	if err != nil {
		t.Fatalf("Start() err = %v", err)
	}

	if err = session.Stop(); err != nil {
		t.Fatalf("Stop() err = %v", err)
	}

	gzipMagic := []byte{0x1f, 0x8b}

	for _, path := range []string{opts.CPUProfile, opts.MemProfile} {
		if content := readFile(t, path); !bytes.HasPrefix(content, gzipMagic) {
			t.Fatalf("%s starts %x, want the gzip magic", path, content[:min(2, len(content))])
		}
	}

	if content := readFile(t, opts.Trace); !bytes.HasPrefix(content, []byte("go 1.")) {
		t.Fatalf("trace starts %q, want the trace header", content[:min(8, len(content))])
	}
}

func TestStartFailures(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent", "out")

	testCases := []struct {
		name    string
		opts    Options
		busy    func(t *testing.T)
		wantErr error
	}{
		{"unwritable cpu path", Options{CPUProfile: missing}, nil, nil},
		{"unwritable heap path", Options{MemProfile: missing}, nil, nil},
		{"unwritable trace path", Options{Trace: missing}, nil, nil},
		{
			"unwritable cpu path after a limit",
			Options{MemLimit: "1GiB", CPUProfile: missing},
			nil,
			nil,
		},
		{"bad memory limit", Options{MemLimit: "lots"}, nil, ErrMemLimit},
		{
			"cpu profile already running",
			Options{CPUProfile: filepath.Join(t.TempDir(), "cpu")},
			func(t *testing.T) {
				t.Helper()

				if err := pprof.StartCPUProfile(&bytes.Buffer{}); err != nil {
					t.Fatalf("start the rival profile: %v", err)
				}

				t.Cleanup(pprof.StopCPUProfile)
			}, nil,
		},
		{
			"trace already running",
			Options{Trace: filepath.Join(t.TempDir(), "trace")},
			func(t *testing.T) {
				t.Helper()

				if err := trace.Start(&bytes.Buffer{}); err != nil {
					t.Fatalf("start the rival trace: %v", err)
				}

				t.Cleanup(trace.Stop)
			}, nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			limit := debug.SetMemoryLimit(-1)

			t.Cleanup(func() { debug.SetMemoryLimit(limit) })

			if testCase.busy != nil {
				testCase.busy(t)
			}

			_, err := Start(testCase.opts)
			if err == nil {
				t.Fatal("Start() err = nil, want a failure")
			}

			if testCase.wantErr != nil && !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Start() err = %v, want %v", err, testCase.wantErr)
			}

			if got := debug.SetMemoryLimit(-1); got != limit {
				t.Fatalf("memory limit = %d after a failed Start, want %d restored", got, limit)
			}
		})
	}
}

func TestMemLimitAppliesUntilStop(t *testing.T) {
	original := debug.SetMemoryLimit(-1)

	t.Cleanup(func() { debug.SetMemoryLimit(original) })

	session, err := Start(Options{MemLimit: "256MiB"})
	if err != nil {
		t.Fatalf("Start() err = %v", err)
	}

	if got := debug.SetMemoryLimit(-1); got != 256<<20 {
		t.Fatalf("memory limit = %d, want %d", got, 256<<20)
	}

	if err = session.Stop(); err != nil {
		t.Fatalf("Stop() err = %v", err)
	}

	if got := debug.SetMemoryLimit(-1); got != original {
		t.Fatalf("memory limit = %d after Stop, want %d", got, original)
	}
}

func TestStartRefusesTwoOutputsInOneFile(t *testing.T) {
	dir := t.TempDir()
	same := filepath.Join(dir, "out")

	testCases := []struct {
		name    string
		opts    Options
		wantErr error
	}{
		{"cpu and heap", Options{CPUProfile: same, MemProfile: same}, ErrSamePath},
		{"cpu and trace", Options{CPUProfile: same, Trace: same}, ErrSamePath},
		{"heap and trace", Options{MemProfile: same, Trace: same}, ErrSamePath},
		{
			"the same file spelled twice",
			Options{CPUProfile: same, Trace: filepath.Join(dir, ".", "out")},
			ErrSamePath,
		},
		{"three different files", Options{
			CPUProfile: filepath.Join(dir, "a"),
			MemProfile: filepath.Join(dir, "b"),
			Trace:      filepath.Join(dir, "c"),
		}, nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			session, err := Start(testCase.opts)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Start() err = %v, want %v", err, testCase.wantErr)
			}

			if err == nil {
				if err = session.Stop(); err != nil {
					t.Fatalf("Stop() err = %v", err)
				}
			}
		})
	}

	if _, err := os.Stat(same); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a refused start created %s (err = %v)", same, err)
	}
}

func TestStopRunsOnce(t *testing.T) {
	original := debug.SetMemoryLimit(-1)

	t.Cleanup(func() { debug.SetMemoryLimit(original) })

	heap := filepath.Join(t.TempDir(), "mem")

	session, err := Start(Options{MemLimit: "256MiB", MemProfile: heap})
	if err != nil {
		t.Fatalf("Start() err = %v", err)
	}

	if err = session.Stop(); err != nil {
		t.Fatalf("first Stop() err = %v", err)
	}

	debug.SetMemoryLimit(1 << 30)

	if err = session.Stop(); err != nil {
		t.Fatalf("second Stop() err = %v, want nil", err)
	}

	if got := debug.SetMemoryLimit(-1); got != 1<<30 {
		t.Fatalf("a second Stop() set the memory limit to %d, want it to do nothing", got)
	}

	var never *Session
	if err = never.Stop(); err != nil {
		t.Fatalf("Stop() on no session err = %v, want nil", err)
	}
}
