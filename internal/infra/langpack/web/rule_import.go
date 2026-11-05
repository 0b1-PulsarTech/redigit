package web

import (
	"cmp"
	"context"
	"regexp"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// importRule makes a file wait for every file of the tree it imports. A
// virtual module waits for both the file declaring its types and the file it
// is compiled from: the first is what svelte-check reads, the second what the
// bundle runs.
type importRule struct{}

func (importRule) Name() string { return "import" }

func (importRule) Edges(ctx context.Context, run *index) ([]entities.Edge, error) {
	var out []entities.Edge

	for _, file := range run.scripts {
		dependent, _ := run.first(file)

		targets, err := run.targets(ctx, file)
		if err != nil {
			return nil, err
		}

		for _, target := range targets {
			if provider, found := run.first(target); found {
				out = append(out, entities.Edge{
					Dependent: dependent,
					Provider:  provider,
					Reason:    entities.ReasonReference,
				})
			}
		}
	}

	return out, nil
}

func (importRule) Anchors(context.Context, *index) ([]entities.Anchor, error) { return nil, nil }

// targets lists, in import order, the files of the tree file imports.
func (run *index) targets(ctx context.Context, file string) ([]string, error) {
	source, err := run.space.read(ctx, file)
	if err != nil {
		return nil, err
	}

	var out []string

	for _, spec := range specifiers(string(source)) {
		target, resolveErr := run.space.resolve(ctx, file, spec)
		if resolveErr != nil {
			return nil, resolveErr
		}

		for _, each := range []string{target, run.virtual[spec]} {
			if each != "" && run.space.files[each] {
				out = append(out, each)
			}
		}
	}

	return out, nil
}

// The scan is textual on purpose: it reads a .svelte file's markup along with
// its script, so a "from '...'" written in prose would count as an import. A
// file holding one is rare, and an extra edge only holds the reader back.
var (
	fromClause   = regexp.MustCompile(`\bfrom\s*['"]([^'"\n]+)['"]`)
	bareImport   = regexp.MustCompile(`\bimport\s*['"]([^'"\n]+)['"]`)
	dynamicCall  = regexp.MustCompile(`\bimport\(\s*['"]([^'"\n]+)['"]\s*\)`)
	visitedRoute = regexp.MustCompile(
		"['\"`}](/[a-z][a-z0-9-]*(?:/(?:[a-z0-9-]+|\\$\\{[^}]*\\}))*)/?['\"`?#]",
	)
)

// specGroup is the capture group each import pattern puts the specifier in.
const specGroup = 1

// specifiers lists, in order of appearance, every module a file names.
func specifiers(source string) []string {
	type found struct {
		at   int
		spec string
	}

	var all []found

	for _, pattern := range []*regexp.Regexp{fromClause, bareImport, dynamicCall} {
		for _, match := range pattern.FindAllStringSubmatchIndex(source, -1) {
			spec := match[specGroup*2 : specGroup*2+2]
			all = append(all, found{at: match[0], spec: source[spec[0]:spec[1]]})
		}
	}

	slices.SortFunc(all, func(a, b found) int { return cmp.Compare(a.at, b.at) })

	out := make([]string, 0, len(all))

	for _, each := range all {
		out = append(out, each.spec)
	}

	return out
}

// interpolated stands for a "${...}" segment, which only a "[param]" route
// segment matches.
const interpolated = "[]"

// visits lists the app paths a test reaches for in a string literal, such as
// "/conversas", `/funil?periodo=` or `${base}/conversas/${id}`.
func visits(source string) []string {
	matches := visitedRoute.FindAllStringSubmatch(source, -1)
	out := make([]string, 0, len(matches))

	for _, match := range matches {
		segments := strings.Split(strings.TrimSuffix(match[1], "/"), "/")
		for index, segment := range segments {
			if strings.HasPrefix(segment, "${") {
				segments[index] = interpolated
			}
		}

		out = append(out, strings.Join(segments, "/"))
	}

	return out
}
