package subjects

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/subjecting"
)

func runDigest(args []string, stdout io.Writer) error {
	manifestFlag := requirement{name: flagManifest}

	err := parse("digest", args, func(set *flag.FlagSet) {
		set.StringVar(&manifestFlag.value, flagManifest, "", "")
	}, &manifestFlag)
	if err != nil {
		return err
	}

	rows, err := loadRows(manifestFlag.value)
	if err != nil {
		return err
	}

	encoder := json.NewEncoder(stdout)

	for _, entry := range subjecting.Digest(rows) {
		if err = encoder.Encode(entry); err != nil {
			return fmt.Errorf("write digest line: %w", err)
		}
	}

	return nil
}
