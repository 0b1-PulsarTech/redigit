package format

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	formatter "github.com/wrapped-owls/redigit/internal/infra/format"
)

type verdict struct {
	raw     []byte
	content []byte
	failure *formatter.Failure
}

func readBlob(dir, handle string) ([]byte, error) {
	if !filepath.IsLocal(handle) || filepath.Base(handle) != handle {
		return nil, fmt.Errorf("handle %q is not a file name", handle)
	}

	content, err := os.ReadFile(filepath.Join(dir, handle)) // #nosec G304 -- handle checked above.
	if err != nil {
		return nil, fmt.Errorf("read blob: %w", err)
	}

	return content, nil
}

func rounds(units []unit) [][]int {
	var out [][]int

	depth := map[string]int{}

	for index, item := range units {
		level := depth[item.path]
		depth[item.path]++

		if level == len(out) {
			out = append(out, nil)
		}

		out[level] = append(out[level], index)
	}

	return out
}

func formatUnits(
	ctx context.Context,
	engine *formatter.Engine,
	blobDir string,
	units []unit,
) ([]verdict, error) {
	verdicts, cache := make([]verdict, len(units)), map[string][]byte{}

	for index, item := range units {
		if _, loaded := cache[item.handle]; !loaded {
			content, err := readBlob(blobDir, item.handle)
			if err != nil {
				return nil, err
			}

			cache[item.handle] = content
		}

		verdicts[index].raw = cache[item.handle]
	}

	// one path once per round, so a failure's path names exactly one blob
	for _, round := range rounds(units) {
		files := make([]formatter.File, len(round))
		for slot, index := range round {
			files[slot] = formatter.File{Path: units[index].path, Content: verdicts[index].raw}
		}

		out, failures := engine.FormatAll(ctx, files)

		failed := make(map[string]*formatter.Failure, len(failures))
		for _, failure := range failures {
			failed[failure.Path] = failure
		}

		for slot, index := range round {
			if failure, bad := failed[units[index].path]; bad {
				verdicts[index].failure = failure

				continue
			}

			verdicts[index].content = out[slot].Content
		}
	}

	return verdicts, nil
}
