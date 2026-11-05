package anchorstate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
)

const (
	StateFile    = "state.json"
	ownedPrefix  = ".redigit-"
	DerivedFile  = ownedPrefix + "continue.json"
	markerFile   = ownedPrefix + "conflicts"
	stageName    = ownedPrefix + "stage"
	oldName      = ownedPrefix + "old"
	tmpPrefix    = ownedPrefix + "tmp-"
	conflictsDir = "conflicts"
	marker       = "redigit-stop-state"
	dirMode      = 0o750
	fileMode     = 0o600
)

var (
	ErrNoState = errors.New("no stop state here")
	ErrCorrupt = errors.New("stop state cannot be read")
	ErrStale   = errors.New("stop state is stale")
	ErrForeign = errors.New("not a redigit stop state")
)

type State struct {
	Marker    string                      `json:"marker"`
	Args      []string                    `json:"args"`
	Commit    string                      `json:"commit"`
	Digest    string                      `json:"digest"`
	Decisions map[string]anchoring.Policy `json:"decisions"`
	Created   bool                        `json:"created"`
	Conflicts []anchoring.Conflict        `json:"conflicts"`
}

func Load(dir string) (State, error) {
	owned, err := inspect(dir)
	if err != nil {
		return State{}, err
	}

	if !owned.hasState {
		return State{}, ErrNoState
	}

	if owned.conflicts == conflictsForeign {
		return State{}, fmt.Errorf("%w: %s has no redigit marker", ErrForeign, conflictsPath(dir))
	}

	return owned.state, owned.stateErr
}

func readState(dir string) (State, error) {
	path := filepath.Join(dir, StateFile)

	raw, err := os.ReadFile(path) // #nosec G304 -- under the state directory.
	if errors.Is(err, fs.ErrNotExist) {
		return State{}, ErrNoState
	}

	if err != nil {
		return State{}, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}

	var state State
	if err = json.Unmarshal(raw, &state); err != nil || state.Marker != marker {
		return State{}, fmt.Errorf("%w: %s", ErrForeign, path)
	}

	if len(state.Args) == 0 {
		return state, fmt.Errorf("%w: %s has no command line", ErrCorrupt, path)
	}

	return state, nil
}

func (state State) Fresh(commit, digest string) error {
	if state.Commit != commit {
		return fmt.Errorf("%w: the revision moved", ErrStale)
	}

	if state.Digest != digest {
		return fmt.Errorf("%w: the config changed", ErrStale)
	}

	return nil
}

func ignoreMissing(err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	return err
}

func DigestFile(path string) (string, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- the config path the run already used.
	if err != nil {
		return "", fmt.Errorf("read the config: %w", err)
	}

	sum := sha256.Sum256(raw)

	return hex.EncodeToString(sum[:]), nil
}
