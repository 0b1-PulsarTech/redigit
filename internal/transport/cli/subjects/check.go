package subjects

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/subjecting"
)

func runCheck(args []string, stdout io.Writer) error {
	const (
		defaultDescription = 72
		defaultLine        = 100
	)

	var limits subjecting.Limits

	manifestFlag := requirement{name: flagManifest}
	subjectsFlag := requirement{name: "subjects"}

	err := parse("check", args, func(set *flag.FlagSet) {
		set.StringVar(&manifestFlag.value, flagManifest, "", "")
		set.StringVar(&subjectsFlag.value, "subjects", "", "")
		set.IntVar(&limits.Description, "max-description", defaultDescription, "")
		set.IntVar(&limits.Line, "max-line", defaultLine, "")
	}, &manifestFlag, &subjectsFlag)
	if err != nil {
		return err
	}

	rows, err := loadRows(manifestFlag.value)
	if err != nil {
		return err
	}

	hand, err := loadSubjects(subjectsFlag.value)
	if err != nil {
		return err
	}

	findings := subjecting.Check(rows, hand, limits)
	encoder := json.NewEncoder(stdout)

	for _, finding := range findings {
		if err = encoder.Encode(finding); err != nil {
			return fmt.Errorf("write finding line: %w", err)
		}
	}

	if len(findings) > 0 {
		return fmt.Errorf("%w: %d", ErrFindings, len(findings))
	}

	return nil
}
