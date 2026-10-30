package slots

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
)

var (
	ErrBadLog = errors.New("malformed git log line")

	_ verifying.Commits = (*Commits)(nil)
)

type Commits struct {
	gitDir string
}

func NewCommits(gitDir string) *Commits { return &Commits{gitDir: gitDir} }

func (commits *Commits) List(ctx context.Context, ref string) ([]verifying.Commit, error) {
	out, err := runGit(ctx, gitCall{gitDir: commits.gitDir, args: []string{
		"log", "--reverse", "--topo-order", "--format=%H%x00%T%x00%s", "--end-of-options", ref,
	}})
	if err != nil {
		return nil, fmt.Errorf("list commits: %w", err)
	}

	return parseCommits(out)
}

func parseCommits(out []byte) ([]verifying.Commit, error) {
	const logFields = 3

	lines := bytes.Split(bytes.TrimSuffix(out, []byte("\n")), []byte("\n"))
	commits := make([]verifying.Commit, 0, len(lines))

	for _, line := range lines {
		if len(line) == 0 {
			continue
		}

		fields := bytes.SplitN(line, []byte{0}, logFields)
		if len(fields) != logFields {
			return nil, ErrBadLog
		}

		commits = append(commits, verifying.Commit{
			Oid: string(fields[0]), Tree: string(fields[1]), Subject: string(fields[2]),
			Position: len(commits) + 1,
		})
	}

	return commits, nil
}
