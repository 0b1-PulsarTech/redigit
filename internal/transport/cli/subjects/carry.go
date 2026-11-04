package subjects

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/subjecting"
)

type report struct {
	Sources     int                  `json:"sources"`
	Identical   int                  `json:"identical"`
	Review      int                  `json:"review"`
	Lost        int                  `json:"lost"`
	Fresh       int                  `json:"fresh"`
	IdenticalPC float64              `json:"identical_pct"`
	ReviewPC    float64              `json:"review_pct"`
	LostPC      float64              `json:"lost_pct"`
	ReviewRows  []subjecting.Match   `json:"review_rows"`
	LostRows    []subjecting.Lost    `json:"lost_rows"`
	Problems    []subjecting.Problem `json:"problems"`
}

func runCarry(args []string, stdout io.Writer) error {
	var reviewPath string

	fromFlag := requirement{name: "from"}
	subjectsFlag := requirement{name: "subjects"}
	manifestFlag := requirement{name: flagManifest}
	outFlag := requirement{name: "out"}

	err := parse("carry", args, func(set *flag.FlagSet) {
		set.StringVar(&fromFlag.value, "from", "", "")
		set.StringVar(&subjectsFlag.value, "subjects", "", "")
		set.StringVar(&manifestFlag.value, flagManifest, "", "")
		set.StringVar(&outFlag.value, "out", "", "")
		set.StringVar(&reviewPath, "review", "", "")
	}, &fromFlag, &subjectsFlag, &manifestFlag, &outFlag)
	if err != nil {
		return err
	}

	carried, err := compute(fromFlag.value, subjectsFlag.value, manifestFlag.value)
	if err != nil {
		return err
	}

	if len(carried.Problems) > 0 {
		return refuse(carried, stdout)
	}

	if err = writeJSON(outFlag.value, keyed(carried.Identical)); err != nil {
		return err
	}

	if reviewPath != "" {
		if err = writeJSON(reviewPath, keyed(carried.Review)); err != nil {
			return err
		}
	}

	if err = json.NewEncoder(stdout).Encode(summarise(carried)); err != nil {
		return fmt.Errorf("write report: %w", err)
	}

	return nil
}

func compute(fromPath, subjectsPath, manifestPath string) (subjecting.Carried, error) {
	old, err := loadDigest(fromPath)
	if err != nil {
		return subjecting.Carried{}, err
	}

	hand, err := loadSubjects(subjectsPath)
	if err != nil {
		return subjecting.Carried{}, err
	}

	rows, err := loadRows(manifestPath)
	if err != nil {
		return subjecting.Carried{}, err
	}

	return subjecting.Carry(old, hand, subjecting.Digest(rows)), nil
}

func keyed(matches []subjecting.Match) map[string]string {
	subjects := make(map[string]string, len(matches))
	for _, match := range matches {
		subjects[subjecting.IDPrefix+match.ID] = match.Subject
	}

	return subjects
}

func summarise(carried subjecting.Carried) report {
	return report{
		Sources:     carried.Sources,
		Identical:   len(carried.Identical),
		Review:      len(carried.Review),
		Lost:        len(carried.Lost),
		Fresh:       carried.Fresh,
		IdenticalPC: percent(len(carried.Identical), carried.Sources),
		ReviewPC:    percent(len(carried.Review), carried.Sources),
		LostPC:      percent(len(carried.Lost), carried.Sources),
		ReviewRows:  orEmpty(carried.Review),
		LostRows:    orEmpty(carried.Lost),
		Problems:    orEmpty(carried.Problems),
	}
}

func percent(part, whole int) float64 {
	const (
		hundred = 100.0
		tenth   = 10.0
	)

	if whole == 0 {
		return 0
	}

	return math.Round(float64(part)/float64(whole)*hundred*tenth) / tenth
}

func refuse(carried subjecting.Carried, stdout io.Writer) error {
	if err := json.NewEncoder(stdout).Encode(summarise(carried)); err != nil {
		return fmt.Errorf("write report: %w", err)
	}

	return fmt.Errorf("%w: %d", ErrFindings, len(carried.Problems))
}
