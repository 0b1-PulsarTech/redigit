package batch

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/rewriting"
)

const stampFields = 2

func AuthorDates(ctx context.Context, repo, ref string) ([]rewriting.Stamp, error) {
	// #nosec G204 -- driving the git CLI is this adapter's purpose.
	cmd := exec.CommandContext(ctx, "git", "-C", repo, "log", "--no-show-signature",
		"--format=%at %ad", "--date=format:%z", "--end-of-options", ref, "--")

	var stderr TailBuffer

	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		return nil, runFailure(ctx, err, &stderr)
	}

	return parseAuthorDates(out)
}

func parseAuthorDates(out []byte) ([]rewriting.Stamp, error) {
	var stamps []rewriting.Stamp

	for line := range bytes.Lines(out) {
		fields := strings.Fields(string(line))
		if len(fields) == 0 {
			continue
		}

		if len(fields) != stampFields {
			return nil, fmt.Errorf("%w: author date line", ErrMalformedLog)
		}

		unix, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: author date stamp: %w", ErrMalformedLog, err)
		}

		stamps = append(stamps, rewriting.Stamp{Unix: unix, Zone: fields[1]})
	}

	return stamps, nil
}
