package passes

import (
	"encoding/json"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/shaping"
)

func Entries() []shaping.Entry {
	return []shaping.Entry{{
		Type: TidyName,
		New: func(raw json.RawMessage) (shaping.Pass, error) {
			tidy, err := NewTidy(raw)
			if err != nil {
				return nil, err
			}

			return tidy, nil
		},
	}, {
		Type: CapName,
		New: func(raw json.RawMessage) (shaping.Pass, error) {
			pass, err := NewCap(raw)
			if err != nil {
				return nil, err
			}

			return pass, nil
		},
	}}
}
