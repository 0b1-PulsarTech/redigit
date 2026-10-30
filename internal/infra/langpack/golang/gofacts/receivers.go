package gofacts

import (
	"go/types"

	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
)

func (scan walker) receiverArgs(recv *types.Named) []facts.TypeRef {
	var out []facts.TypeRef

	// A method has no type parameters of its own; its receiver instance carries the arguments.
	if recv != nil {
		for arg := range recv.TypeArgs().Types() {
			out = append(out, scan.refs.ref(arg))
		}
	}

	return out
}

func receiverOf(fn *types.Func) (*types.Named, bool) {
	sig, isSig := fn.Type().(*types.Signature)
	if !isSig || sig.Recv() == nil {
		return nil, false
	}

	typ := types.Unalias(sig.Recv().Type())
	if pointer, isPointer := typ.(*types.Pointer); isPointer {
		typ = types.Unalias(pointer.Elem())
	}

	named, _ := typ.(*types.Named)

	return named, true
}
