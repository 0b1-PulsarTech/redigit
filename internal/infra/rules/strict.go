package rules

import (
	"bytes"
	"encoding/json"
	"fmt"
)

func Expected(kind string) error { return fmt.Errorf("expected %s", kind) }

func DecodeStrict(raw json.RawMessage, dst any, kind string) error {
	// A null decodes into any pointer, slice or map without error; it is no value here.
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return Expected(kind)
	}

	if err := json.Unmarshal(raw, dst); err != nil {
		return Expected(kind)
	}

	return nil
}
