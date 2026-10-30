package bootstrap

import (
	"context"

	"github.com/wrapped-owls/redigit/internal/infra/rules"
)

func ruleHook(context.Context, *Config, stageEnv, *stages) error {
	return nil
}

func ruleConstructors() map[string]rules.Constructor {
	// No rule type ships yet: each idiom pack registers its own constructor here.
	return map[string]rules.Constructor{}
}
