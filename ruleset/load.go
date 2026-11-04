// Package ruleset loads lint rules from YAML files. Their checks are CEL expressions.
package ruleset

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"

	"cel.dev/cel-go/cel"
	yamlv3 "go.yaml.in/yaml/v3"
	"sigs.k8s.io/yaml"

	"github.com/rebEllieous/ocm-lint-solar/lint"
)

// Load parses and compiles rule files, sorted by ID. A rule replaces one with the
// same ID from an earlier file. An ID in a pack that an earlier file used, but
// matching nothing, is probably a mistyped override and gets a warning. err means
// the files can't be trusted and nothing should run.
func Load(paths []string) (rules []lint.Rule, warnings []string, err error) {
	if len(paths) == 0 {
		return nil, nil, errors.New("no rule files")
	}
	env, err := newEnv()
	if err != nil {
		return nil, nil, err
	}
	merged := map[string]lint.Rule{}
	earlier := map[string]bool{}
	for _, path := range paths {
		rs, err := loadFile(env, path)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", path, err)
		}
		for _, r := range rs {
			if _, exists := merged[r.id]; !exists && earlier[pack(r.id)] {
				warnings = append(warnings, fmt.Sprintf("%s: rule %q overrides nothing in pack %q; use your own pack for a new rule", path, r.id, pack(r.id)))
			}
			merged[r.id] = r
		}
		for _, r := range rs {
			earlier[pack(r.id)] = true
		}
	}
	return sorted(merged), warnings, nil
}

func pack(id string) string {
	p, _, _ := strings.Cut(id, "/")
	return p
}

func loadFile(env *cel.Env, path string) ([]*rule, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := oneDocument(b); err != nil {
		return nil, err
	}
	var file struct {
		Rules []ruleSpec `json:"rules"`
	}
	if err := yaml.UnmarshalStrict(b, &file); err != nil {
		return nil, err
	}
	if len(file.Rules) == 0 {
		return nil, errors.New("no rules")
	}
	seen := map[string]bool{}
	var out []*rule
	for _, s := range file.Rules {
		if seen[s.ID] {
			return nil, fmt.Errorf("duplicate rule %q", s.ID)
		}
		seen[s.ID] = true
		r, err := compile(env, s)
		if err != nil {
			return nil, fmt.Errorf("rule %q: %w", s.ID, err)
		}
		out = append(out, r)
	}
	return out, nil
}

// oneDocument rejects a second YAML document, which the decoder would silently
// drop along with its rules.
func oneDocument(b []byte) error {
	dec := yamlv3.NewDecoder(bytes.NewReader(b))
	docs := 0
	for {
		var doc any
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if doc != nil {
			docs++
		}
		if docs > 1 {
			return errors.New("more than one YAML document; a rule file holds one")
		}
	}
}

func sorted(m map[string]lint.Rule) []lint.Rule {
	out := slices.Collect(maps.Values(m))
	slices.SortFunc(out, func(a, b lint.Rule) int { return cmp.Compare(a.ID(), b.ID()) })
	return out
}

// Skip removes rules by ID. An ID that isn't loaded is an error, since it is
// probably a typo that leaves the rule running.
func Skip(rules []lint.Rule, ids []string) ([]lint.Rule, error) {
	for _, id := range ids {
		if !slices.ContainsFunc(rules, func(r lint.Rule) bool { return r.ID() == id }) {
			return nil, fmt.Errorf("unknown rule %q", id)
		}
	}
	return slices.DeleteFunc(slices.Clone(rules), func(r lint.Rule) bool { return slices.Contains(ids, r.ID()) }), nil
}
