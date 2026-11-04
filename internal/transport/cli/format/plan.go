package format

import (
	"github.com/wrapped-owls/redigit/internal/transport/cli/manifest"
)

type (
	spot struct{ row, blob int }

	unit struct{ handle, path string }

	plan struct {
		units     []unit
		blobs     int
		handles   int
		finals    int
		unclaimed int
	}
)

func finalSpots(rows []manifest.Row) map[spot]struct{} {
	best, position := map[string]spot{}, map[string]int{}

	for rowIndex, row := range rows {
		for blobIndex, blob := range row.Blobs {
			if seen, found := position[blob.Path]; found && seen > row.Position {
				continue
			}

			position[blob.Path], best[blob.Path] = row.Position, spot{rowIndex, blobIndex}
		}
	}

	finals := make(map[spot]struct{}, len(best))
	for _, last := range best {
		finals[last] = struct{}{}
	}

	return finals
}

func regular(blob manifest.Blob) bool {
	return blob.Handle != "" && (blob.Mode == "100644" || blob.Mode == "100755")
}

func collect(rows []manifest.Row, finals map[spot]struct{}, set tools) plan {
	var work plan

	handles, units := map[string]struct{}{}, map[unit]struct{}{}

	for rowIndex, row := range rows {
		for blobIndex, blob := range row.Blobs {
			work.blobs++

			handles[blob.Handle] = struct{}{}

			if _, last := finals[spot{rowIndex, blobIndex}]; last {
				work.finals++

				continue
			}

			item := unit{handle: blob.Handle, path: blob.Path}
			if _, dup := units[item]; dup || !regular(blob) {
				continue
			}

			units[item] = struct{}{}

			if set.covers(blob.Path) {
				work.units = append(work.units, item)
			} else {
				work.unclaimed++
			}
		}
	}

	work.handles = len(handles)

	return work
}
