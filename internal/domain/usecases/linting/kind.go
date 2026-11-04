package linting

import (
	"bytes"
	"path"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

type Kind int

const (
	KindOther Kind = iota
	KindSource
	KindGenerated
)

func Classify(file string, source []byte) Kind {
	// The marker decides first; the name list is only for what can carry none.
	base := strings.ToLower(path.Base(file))

	if indexing.IsGenerated(file, source) || isMarkerless(base) {
		return KindGenerated
	}

	if slices.Contains(sourceExtensions(), path.Ext(base)) {
		return KindSource
	}

	return KindOther
}

func IsBinary(source []byte) bool {
	return bytes.IndexByte(source, 0) >= 0
}

func sourceExtensions() []string {
	return []string{".go", ".dart", ".ts", ".tsx", ".js", ".jsx", ".svelte", ".sql", ".py", ".rs"}
}

func isMarkerless(base string) bool {
	return slices.Contains([]string{"package-lock.json", "pnpm-lock.yaml"}, base) ||
		strings.HasSuffix(base, ".min.js")
}
