package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
)

var errVirtualForm = errors.New("--virtual wants ID=PATH")

// addVirtual reads one --virtual flag into cfg; the flag repeats, one virtual
// module per occurrence.
func addVirtual(cfg *bootstrap.Config) func(string) error {
	return func(value string) error {
		id, file, found := strings.Cut(value, "=")
		if !found || strings.TrimSpace(id) == "" || strings.TrimSpace(file) == "" {
			return fmt.Errorf("%w, got %q", errVirtualForm, value)
		}

		if cfg.Virtual == nil {
			cfg.Virtual = map[string]string{}
		}

		cfg.Virtual[strings.TrimSpace(id)] = strings.TrimSpace(file)

		return nil
	}
}
