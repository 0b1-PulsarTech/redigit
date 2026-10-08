package emitting

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// derivation is the state of the derived files across one Emit: which paths
// are derived and by whom, what each input last rendered to, and what each
// output last held. Nothing outside the run sees it.
type derivation struct {
	outputs map[string]Deriver
	inputs  map[string][]byte
	last    map[string][]byte
}

// owns reports whether path is a derived output. It is nil-safe: a run with no
// deriver has no derivation.
func (d *derivation) owns(path string) bool {
	if d == nil {
		return false
	}

	_, derived := d.outputs[path]

	return derived
}

// deriving returns the emitter wired for a run of plan: the derived outputs the
// plan holds, and a sink that remembers what the files feeding them rendered
// to. The emitter itself stays reusable.
func (e Emitter) deriving(plan entities.Plan) Emitter {
	state := &derivation{
		outputs: map[string]Deriver{},
		inputs:  map[string][]byte{},
		last:    map[string][]byte{},
	}

	for _, commit := range plan.Commits {
		for _, key := range commit.Decls {
			for _, deriver := range e.derivers {
				if deriver.Output(key.File) {
					state.outputs[key.File] = deriver
				}
			}
		}
	}

	if len(state.outputs) == 0 {
		return e
	}

	e.derived = state
	e.sink = recordingSink{Sink: e.sink, state: state}

	return e
}

// recordingSink keeps the content of every file a derived output reads.
type recordingSink struct {
	Sink

	state *derivation
}

func (r recordingSink) Write(
	ctx context.Context,
	path string,
	content []byte,
	mode FileMode,
) (Handle, error) {
	for output, deriver := range r.state.outputs {
		if deriver.Feeds(output, path) {
			r.state.inputs[path] = content
		}
	}

	handle, err := r.Sink.Write(ctx, path, content, mode)
	if err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}

	return handle, nil
}

// deriveTouched adds a blob for every derived output this commit changes: one
// whose inputs were touched, or one whose own declaration is born here. An
// output with no input yet is not written, and one whose content did not
// change is not written again.
func (e Emitter) deriveTouched(
	ctx context.Context,
	touched map[string]bool,
	blobs []Blob,
	cache map[string]sourceFile,
) ([]Blob, error) {
	if e.derived == nil {
		return blobs, nil
	}

	for _, output := range slices.Sorted(maps.Keys(e.derived.outputs)) {
		blob, written, err := e.deriveOne(ctx, output, touched, cache)
		if err != nil {
			return nil, err
		}

		if written {
			blobs = append(blobs, blob)
		}
	}

	slices.SortFunc(blobs, func(a, b Blob) int { return cmp.Compare(a.Path, b.Path) })

	return blobs, nil
}

func (e Emitter) deriveOne(
	ctx context.Context,
	output string,
	touched map[string]bool,
	cache map[string]sourceFile,
) (Blob, bool, error) {
	state, deriver := e.derived, e.derived.outputs[output]
	inputs := map[string][]byte{}
	changed := touched[output]

	for path, content := range state.inputs {
		if !deriver.Feeds(output, path) {
			continue
		}

		inputs[path] = content
		changed = changed || touched[path]
	}

	if len(inputs) == 0 || !changed {
		return Blob{}, false, nil
	}

	content, err := deriver.Derive(output, inputs)
	if err != nil {
		return Blob{}, false, fmt.Errorf("derive %s: %w", output, err)
	}

	if bytes.Equal(content, state.last[output]) && !touched[output] {
		return Blob{}, false, nil
	}

	file, cached := cache[output]
	if !cached {
		final, mode, readErr := e.source.Read(ctx, output)
		if readErr != nil {
			return Blob{}, false, fmt.Errorf("read %s: %w", output, readErr)
		}

		file = sourceFile{content: final, mode: mode}
		cache[output] = file
	}

	handle, err := e.sink.Write(ctx, output, content, file.mode)
	if err != nil {
		return Blob{}, false, fmt.Errorf("write %s: %w", output, err)
	}

	state.last[output] = content

	return Blob{Path: output, Mode: file.mode, Handle: handle}, true, nil
}
