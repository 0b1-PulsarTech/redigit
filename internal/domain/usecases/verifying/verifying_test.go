package verifying

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	errBoom         = errors.New("boom")
	_       Trees   = (*fakeTrees)(nil)
	_       Builder = (*fakeBuilder)(nil)
	_       Cache   = (*fakeCache)(nil)
)

type (
	fakeCommits struct {
		list []Commit
		err  error
	}

	fakeModules struct {
		byOid  map[string][]Module
		strays map[string][]string
		err    error
	}

	fakeTrees struct {
		mutex sync.Mutex
		err   error
		slots map[int][]int
		calls int

		closes   int
		closeErr error
	}

	fakeBuilder struct {
		mutex       sync.Mutex
		failing     map[string]string
		identityErr error
		checkErr    error
		onCheck     func(ctx context.Context) error
		checked     []string
	}

	fakeCache struct {
		mutex  sync.Mutex
		stored map[string]Outcome
		getErr error
		putErr error
		puts   int
	}
)

func (fake fakeCommits) List(context.Context, string) ([]Commit, error) {
	return fake.list, fake.err
}

func (fake fakeModules) List(_ context.Context, commit Commit) (Listing, error) {
	return Listing{Modules: fake.byOid[commit.Oid], Strays: fake.strays[commit.Oid]}, fake.err
}

func (fake *fakeTrees) Materialise(_ context.Context, slot int, commit Commit) (string, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	fake.calls++

	if fake.slots == nil {
		fake.slots = map[int][]int{}
	}

	fake.slots[slot] = append(fake.slots[slot], commit.Position)

	return commit.Oid, fake.err
}

func (fake *fakeTrees) Close(context.Context) error {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	fake.closes++

	return fake.closeErr
}

func (fake *fakeBuilder) Identity(context.Context, Unit) (string, error) {
	return "identity", fake.identityErr
}

func (fake *fakeBuilder) Check(ctx context.Context, slotDir string, unit Unit) (Outcome, error) {
	fake.mutex.Lock()
	fake.checked = append(fake.checked, slotDir+"/"+unit.Dir)
	tail, isFailing := fake.failing[slotDir+"/"+unit.Dir]
	fake.mutex.Unlock()

	if fake.onCheck != nil {
		if err := fake.onCheck(ctx); err != nil {
			return Outcome{}, err
		}
	}

	if fake.checkErr != nil {
		return Outcome{}, fake.checkErr
	}

	if isFailing {
		return Outcome{IsFailed: true, Step: "go build", Tail: tail}, nil
	}

	return Outcome{Step: "go vet"}, nil
}

func (fake *fakeCache) Get(_ context.Context, key string) (Outcome, bool, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	outcome, isHit := fake.stored[key]

	return outcome, isHit, fake.getErr
}

func (fake *fakeCache) Put(_ context.Context, key string, outcome Outcome) error {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()

	fake.puts++

	if fake.putErr != nil {
		return fake.putErr
	}

	fake.stored[key] = outcome

	return nil
}

type rig struct {
	sweep   Sweep
	trees   *fakeTrees
	builder *fakeBuilder
	cache   *fakeCache
}

func newRig(count int, modules []Module) *rig {
	commits := make([]Commit, count)
	byOid := map[string][]Module{}

	for idx := range commits {
		oid := fmt.Sprintf("c%02d", idx+1)
		commits[idx] = Commit{Oid: oid, Subject: "s " + oid, Position: idx + 1}

		for _, module := range modules {
			module.Subtree += oid
			byOid[oid] = append(byOid[oid], module)
		}
	}

	built := &rig{
		trees:   &fakeTrees{},
		builder: &fakeBuilder{failing: map[string]string{}},
		cache:   &fakeCache{stored: map[string]Outcome{}},
	}
	built.sweep = Sweep{
		Commits: fakeCommits{list: commits},
		Trees:   built.trees,
		Modules: fakeModules{byOid: byOid},
		Builder: built.builder,
		Cache:   built.cache,
	}

	return built
}

func collect(sweep Sweep, opts Options) ([]Row, Summary, error) {
	var rows []Row

	summary, err := sweep.Run(context.Background(), opts, func(row Row) error {
		rows = append(rows, row)

		return nil
	})

	return rows, summary, err
}
