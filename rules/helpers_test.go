package rules_test

import (
	"context"
	"sync"
	"testing"

	"github.com/rebEllieous/ocm-lint-solar/lint"
	"github.com/rebEllieous/ocm-lint-solar/ruleset"
)

var load = sync.OnceValues(func() ([]lint.Rule, error) {
	rules, _, err := ruleset.Load([]string{"core.yaml", "air-gap.yaml", "solar.yaml"})
	return rules, err
})

// run checks one rule from the rule files on a target.
func run(t *testing.T, id string, tg *lint.Target) []lint.Finding {
	t.Helper()
	rules, err := load()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rules {
		if r.ID() == id {
			f, err := r.Check(context.Background(), tg)
			if err != nil {
				t.Fatalf("%s: %v", id, err)
			}
			return f
		}
	}
	t.Fatalf("rule %s not in the rule files", id)
	return nil
}
