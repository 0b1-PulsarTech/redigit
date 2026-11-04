package anchorprobe

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
)

var (
	ErrEdit  = errors.New("edit does not fit the file")
	ErrUnfit = errors.New("pre-image cannot be rendered")
)

func splice(src []byte, edits []anchoring.Edit) ([]byte, error) {
	sorted := slices.Clone(edits)
	slices.SortFunc(sorted, func(left, right anchoring.Edit) int {
		return cmp.Compare(left.Span.Start, right.Span.Start)
	})

	sorted = slices.CompactFunc(sorted, func(left, right anchoring.Edit) bool {
		return left.File == right.File && left.Span == right.Span && left.Text == right.Text
	})

	var out bytes.Buffer

	cursor := 0

	for _, edit := range sorted {
		span := edit.Span
		if span.Start < cursor || span.End < span.Start || span.End > len(src) {
			return nil, fmt.Errorf("%w: %s [%d,%d) of %d bytes, cursor %d",
				ErrEdit, edit.File, span.Start, span.End, len(src), cursor)
		}

		out.Write(src[cursor:span.Start])
		out.WriteString(edit.Text)

		cursor = span.End
	}

	out.Write(src[cursor:])

	return out.Bytes(), nil
}

func affectedBy(units []verifying.Unit, files []string) []verifying.Unit {
	var out []verifying.Unit

	for _, unit := range units {
		dirs := []string{unit.Dir}
		for _, dep := range unit.Deps {
			dirs = append(dirs, dep.Dir)
		}

		if slices.ContainsFunc(files, func(file string) bool {
			return slices.ContainsFunc(dirs, func(dir string) bool { return within(dir, file) })
		}) {
			out = append(out, unit)
		}
	}

	return out
}

func within(dir, file string) bool {
	return dir == "" || dir == "." || strings.HasPrefix(file, strings.TrimSuffix(dir, "/")+"/")
}
