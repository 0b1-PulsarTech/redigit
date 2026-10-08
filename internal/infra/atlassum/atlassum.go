package atlassum

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/emitting"
)

// Deriver writes the atlas.sum beside a directory of migrations. The file
// holds a running hash of every migration in name order, so one statement
// added to one migration changes its line and every line after it; the final
// file is right for the final tree and wrong for each tree before it.
type Deriver struct{}

var _ emitting.Deriver = Deriver{}

const (
	sumFile   = "atlas.sum"
	migration = ".sql"
)

func (Deriver) Output(file string) bool { return path.Base(file) == sumFile }

func (Deriver) Feeds(output, input string) bool {
	return path.Dir(input) == path.Dir(output) && strings.HasSuffix(input, migration)
}

// Derive hashes the migrations the way `atlas migrate hash` does: each
// file's hash covers its name and bytes and every file before it, and the
// header hashes the names and file hashes together.
func (Deriver) Derive(_ string, inputs map[string][]byte) ([]byte, error) {
	names := make([]string, 0, len(inputs))
	for file := range inputs {
		names = append(names, file)
	}

	slices.SortFunc(
		names,
		func(a, b string) int { return strings.Compare(path.Base(a), path.Base(b)) },
	)

	var (
		running = sha256.New()
		header  = sha256.New()
		lines   strings.Builder
	)

	for _, file := range names {
		name := path.Base(file)

		running.Write([]byte(name))
		running.Write(inputs[file])

		hash := base64.StdEncoding.EncodeToString(running.Sum(nil))

		header.Write([]byte(name))
		header.Write([]byte(hash))
		fmt.Fprintf(&lines, "%s h1:%s\n", name, hash)
	}

	return fmt.Appendf(
		nil,
		"h1:%s\n%s",
		base64.StdEncoding.EncodeToString(header.Sum(nil)),
		lines.String(),
	), nil
}
