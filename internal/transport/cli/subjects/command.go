package subjects

import (
	"context"
	"fmt"
	"io"

	"github.com/wrapped-owls/redigit/internal/transport/cli/write"
)

const Usage = ""

func Run(_ context.Context, _ []string, _ io.Writer) error {
	return fmt.Errorf("%w: arrives in slice 19", write.ErrUsage)
}
