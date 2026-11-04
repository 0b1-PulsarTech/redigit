package facts

import (
	"fmt"
	"path"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type TypeRef struct {
	Key  entities.DeclKey
	Name string
}

func (ref TypeRef) IsZero() bool {
	return ref.Key.IsZero() && ref.Name == ""
}

type Call struct {
	Site      entities.DeclKey
	Callee    string
	CalleeKey entities.DeclKey
	Receiver  TypeRef
	TypeArgs  []TypeRef
	FuncArgs  []entities.DeclKey
	Literals  []string
	Within    *Call // the call a func literal holding this call was passed to
}

func (call Call) Short() string {
	at := strings.LastIndex(call.Callee, ".")
	// The last element of an import path may hold dots, so the name is read from the right:
	// a method is Type.Method, a function is Func.
	if !call.Receiver.IsZero() {
		at = strings.LastIndex(call.Callee[:max(at, 0)], ".")
	}

	return call.Callee[at+1:]
}

func (call Call) Matches(pattern string) (bool, error) {
	matched := false

	for _, name := range [...]string{call.Short(), call.Callee} {
		found, err := path.Match(pattern, name)
		if err != nil {
			return false, fmt.Errorf("match call pattern %q: %w", pattern, err)
		}

		matched = matched || found
	}

	return matched, nil
}
