package pinrule

import (
	"encoding/json"

	"github.com/wrapped-owls/redigit/internal/infra/rules"
)

type Rule struct {
	name  string
	decl  pattern
	after pattern
}

var _ rules.Globber = (*Rule)(nil)

func Constructor(name string) rules.Constructor {
	return func(raw json.RawMessage) (rules.Rule, error) {
		rule, err := decode(raw)
		if err != nil {
			return nil, err
		}

		rule.name = name

		return &rule, nil
	}
}

func (rule *Rule) Type() string { return rule.name }

func (rule *Rule) Globs() []string {
	return []string{rule.decl.file.text, rule.after.file.text}
}
