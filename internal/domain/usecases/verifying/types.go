package verifying

import (
	"context"
	"errors"
)

var (
	ErrBuilder = errors.New("builder failed")
	ErrInvalid = errors.New("invalid sweep options")
)

type Status string

const (
	StatusPass  Status = "pass"
	StatusFail  Status = "fail"
	StatusEmpty Status = "empty"
)

type (
	Commit struct {
		Oid      string
		Tree     string
		Subject  string
		Position int
	}

	Module struct {
		Dir       string
		Path      string
		Language  string
		Subtree   string
		GoVersion string
		Requires  []string
		Workspace string
		Locals    []Local
	}

	Local struct {
		Dir     string
		Subtree string
	}

	Listing struct {
		Modules []Module
		Strays  []string
	}

	Unit struct {
		Module

		Deps []Module
	}

	Outcome struct {
		IsFailed  bool
		IsTimeout bool
		Step      string
		Tail      string
	}

	Row struct {
		Position int
		Commit   string
		Subject  string
		Module   string
		Language string
		Status   Status
		Step     string
		Tail     string
		IsWip    bool
		IsCached bool
		Millis   int64
	}

	Options struct {
		Ref      string
		From     int
		To       int
		Stride   int
		Parallel int
		Exclude  []string
	}

	Summary struct {
		Commits        int
		Checked        int
		Passed         int
		Failed         int
		WipFailed      int
		Empty          int
		Cached         int
		OverheadMicros int64
	}

	Commits interface {
		List(ctx context.Context, ref string) ([]Commit, error)
	}

	Trees interface {
		Materialise(ctx context.Context, slot int, commit Commit) (string, error)
		Close(ctx context.Context) error
	}

	Modules interface {
		List(ctx context.Context, commit Commit) (Listing, error)
	}

	Builder interface {
		Identity(ctx context.Context, unit Unit) (string, error)
		Check(ctx context.Context, slotDir string, unit Unit) (Outcome, error)
	}

	Cache interface {
		Get(ctx context.Context, key string) (Outcome, bool, error)
		Put(ctx context.Context, key string, outcome Outcome) error
	}
)
