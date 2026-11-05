package lineageconf

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
	"github.com/wrapped-owls/redigit/internal/infra/rules"
)

var errNoSection = errors.New("the config has no lineage section")

func Decide(
	config []byte,
	decisions map[string]anchoring.Policy,
	rest anchoring.Policy,
) ([]byte, error) {
	var file map[string]json.RawMessage
	if err := json.Unmarshal(config, &file); err != nil {
		return nil, fmt.Errorf("read the config: %w", err)
	}

	if len(file["lineage"]) == 0 {
		return nil, errNoSection
	}

	section, err := Decode(file["lineage"])
	if err != nil {
		return nil, err
	}

	for idx, spec := range section.Anchors {
		for rev, policy := range decisions {
			if strings.HasPrefix(rev, spec.Rev) {
				section.Anchors[idx].Policy = policy
			}
		}

		if rest != "" && section.Anchors[idx].Policy == anchoring.PolicyStop {
			section.Anchors[idx].Policy = rest
		}
	}

	if rest != "" && section.Policy == anchoring.PolicyStop {
		section.Policy = rest
	}

	if file["lineage"], err = json.Marshal(section); err != nil {
		return nil, fmt.Errorf("write the lineage section: %w", err)
	}

	out, err := json.Marshal(file)
	if err != nil {
		return nil, fmt.Errorf("write the config: %w", err)
	}

	// The run reads the derived file through the same bounded reader as any config.
	const maxBytes = 1 << 20
	if len(out) > maxBytes {
		return nil, fmt.Errorf("derive the config: %w", rules.ErrTooLarge)
	}

	return out, nil
}
