package anchorprobe

import (
	"encoding/json"
	"fmt"
	"maps"

	"github.com/wrapped-owls/redigit/internal/infra/checkrun"
	"github.com/wrapped-owls/redigit/internal/infra/verifyconf"
)

type verification struct {
	languages map[string]checkrun.Language
	markers   map[string]string
	exclude   []string
}

func configure(tags []string, raw json.RawMessage) (verification, error) {
	file := verifyconf.Seed()

	if len(raw) > 0 {
		decoded, err := verifyconf.Decode(raw, file)
		if err != nil {
			return verification{}, fmt.Errorf("read the verify section: %w", err)
		}

		file = decoded
	}

	// A tag given on the command line wins; a redefined go language keeps its own steps.
	if len(tags) == 0 {
		tags = file.Tags
	}

	languages := map[string]checkrun.Language{verifyconf.GoName: checkrun.GoDefault(tags)}
	maps.Copy(languages, file.Languages)

	return verification{languages: languages, markers: file.Markers, exclude: file.Exclude}, nil
}
