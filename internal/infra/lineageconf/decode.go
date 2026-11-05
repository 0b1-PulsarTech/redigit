package lineageconf

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
)

type Section struct {
	Follow  bool             `json:"follow"`
	Policy  anchoring.Policy `json:"policy"`
	Anchors []anchoring.Spec `json:"anchors"`
}

func Decode(raw json.RawMessage) (Section, error) {
	section := Section{Follow: true}
	if len(raw) == 0 {
		return section, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&section); err != nil {
		return Section{}, fmt.Errorf("lineage section: %w", err)
	}

	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return Section{}, errors.New("lineage section: trailing data")
	}

	if err := anchoring.Validate(section.Anchors, section.Policy); err != nil {
		return Section{}, fmt.Errorf("lineage section: %w", err)
	}

	return section, nil
}

func (section Section) IsStopping() bool {
	return section.Policy == anchoring.PolicyStop ||
		slices.ContainsFunc(section.Anchors, func(spec anchoring.Spec) bool {
			return spec.Policy == anchoring.PolicyStop
		})
}
