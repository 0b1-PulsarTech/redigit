package web

import (
	"encoding/json"
	"path"
	"strings"
)

const (
	packageJSON  = "package.json"
	tsconfigJSON = "tsconfig.json"
)

// manifest is what a package.json says about resolving a specifier: its
// name, its "exports" for a reader outside it, its "imports" for "#" names.
type manifest struct {
	Name    string                     `json:"name"`
	Exports json.RawMessage            `json:"exports"`
	Imports map[string]json.RawMessage `json:"imports"`
}

// tsconfig carries the "paths" a TypeScript reader resolves first, which is
// how a virtual module gets the file declaring its types.
type tsconfig struct {
	CompilerOptions struct {
		Paths map[string][]string `json:"paths"`
	} `json:"compilerOptions"`
}

// target reads one exports or imports value: a path, or a conditions object
// whose first condition a bundler for Svelte would take.
func target(raw json.RawMessage) string {
	var direct string
	if json.Unmarshal(raw, &direct) == nil {
		return direct
	}

	var conditions map[string]json.RawMessage
	if json.Unmarshal(raw, &conditions) != nil {
		return ""
	}

	for _, condition := range []string{"svelte", "import", "default", "types"} {
		if found, ok := conditions[condition]; ok {
			return target(found)
		}
	}

	return ""
}

// exportsMap normalises "exports" to subpath keys: a lone string or a
// conditions object is the "." entry.
func (m manifest) exportsMap() map[string]json.RawMessage {
	var subpaths map[string]json.RawMessage
	if json.Unmarshal(m.Exports, &subpaths) == nil && hasSubpaths(subpaths) {
		return subpaths
	}

	if len(m.Exports) == 0 {
		return nil
	}

	return map[string]json.RawMessage{".": m.Exports}
}

func hasSubpaths(entries map[string]json.RawMessage) bool {
	for key := range entries {
		if strings.HasPrefix(key, ".") {
			return true
		}
	}

	return false
}

// match finds key among patterns, exactly or through one "*" wildcard, the
// longest prefix winning as Node's resolver has it, and returns the pattern's
// value with the wildcard filled in.
func match[V any](patterns map[string]V, key string, fill func(V, string) string) (string, bool) {
	if value, found := patterns[key]; found {
		return fill(value, ""), true
	}

	best, isFound := "", false

	for pattern := range patterns {
		prefix, suffix, isWild := strings.Cut(pattern, "*")
		fits := isWild && strings.HasPrefix(key, prefix) && strings.HasSuffix(key, suffix) &&
			len(key) >= len(prefix)+len(suffix)
		bestPrefix, _, _ := strings.Cut(best, "*")

		if fits && (!isFound || len(prefix) > len(bestPrefix)) {
			best, isFound = pattern, true
		}
	}

	if !isFound {
		return "", false
	}

	prefix, suffix, _ := strings.Cut(best, "*")

	return fill(patterns[best], key[len(prefix):len(key)-len(suffix)]), true
}

func fillRaw(value json.RawMessage, star string) string {
	return strings.ReplaceAll(target(value), "*", star)
}

func fillPaths(value []string, star string) string {
	if len(value) == 0 {
		return ""
	}

	return strings.ReplaceAll(value[0], "*", star)
}

// nearest walks up from dir to the first directory index holds.
func nearest[V any](index map[string]V, dir string) (string, V, bool) {
	for {
		if found, ok := index[dir]; ok {
			return dir, found, true
		}

		if dir == "." || dir == "/" || dir == "" {
			var zero V

			return "", zero, false
		}

		dir = path.Dir(dir)
	}
}
