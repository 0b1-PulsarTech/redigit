package anchorstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrUnsafe = errors.New("a rev cannot name a directory")

type saver struct{ hook func(step string) error }

func Save(dir string, state State) error { return saver{}.save(dir, state) }

func (store saver) save(dir string, state State) error {
	for _, conflict := range state.Conflicts {
		if !filepath.IsLocal(conflict.Rev) || strings.ContainsAny(conflict.Rev, "\r\n") ||
			strings.ContainsRune(conflict.Rev, filepath.Separator) {
			return fmt.Errorf("%w: %q", ErrUnsafe, conflict.Rev)
		}
	}

	prior, err := inspect(dir)
	if err != nil {
		return err
	}

	if err = prior.refuseConflicts(dir); err != nil {
		return err
	}

	state.Marker, state.Created = marker, prior.state.Created
	if !prior.exists {
		state.Created = true

		if err = os.MkdirAll(dir, dirMode); err != nil {
			return fmt.Errorf("make the state directory: %w", err)
		}
	}

	names, err := reservedIn(dir)
	if err == nil {
		err = clearOwned(dir, names)
	}

	if err != nil {
		return fmt.Errorf("clear what an earlier save left: %w", err)
	}

	if err = store.stage(dir, state); err != nil {
		return err
	}

	if err = store.swap(dir); err != nil {
		return err
	}

	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode the state: %w", err)
	}

	return store.writeAtomic(filepath.Join(dir, StateFile), raw)
}

func WriteConfig(dir string, content []byte) error {
	return saver{}.writeAtomic(filepath.Join(dir, DerivedFile), content)
}
