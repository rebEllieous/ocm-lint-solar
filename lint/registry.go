package lint

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Registry holds rules by ID. A pack is the prefix of a rule ID.
type Registry struct {
	rules map[string]Rule
}

// Default is the registry packs register into from init().
var Default = &Registry{}

func Register(r Rule) { Default.Register(r) }

func (g *Registry) Register(r Rule) {
	pack, name, ok := strings.Cut(r.ID(), "/")
	if !ok || pack == "" || name == "" {
		panic(fmt.Sprintf("rule ID %q is not <pack>/<name>", r.ID()))
	}
	if g.rules == nil {
		g.rules = map[string]Rule{}
	}
	if _, dup := g.rules[r.ID()]; dup {
		panic(fmt.Sprintf("duplicate rule %q", r.ID()))
	}
	g.rules[r.ID()] = r
}

// All returns every rule sorted by ID.
func (g *Registry) All() []Rule { return sorted(g.rules) }

// Select resolves --rule and --skip values. core is always included.
func (g *Registry) Select(include, skip []string) ([]Rule, error) {
	sel := map[string]Rule{}
	addPack := func(pack string) bool {
		found := false
		for id, r := range g.rules {
			if p, _, _ := strings.Cut(id, "/"); p == pack {
				sel[id] = r
				found = true
			}
		}
		return found
	}

	addPack("core")
	for _, s := range include {
		if strings.Contains(s, "/") {
			r, ok := g.rules[s]
			if !ok {
				return nil, fmt.Errorf("unknown rule %q", s)
			}
			sel[s] = r
			continue
		}
		if !addPack(s) {
			return nil, fmt.Errorf("unknown pack %q", s)
		}
	}
	for _, s := range skip {
		if _, ok := g.rules[s]; !ok {
			return nil, fmt.Errorf("unknown rule %q", s)
		}
		delete(sel, s)
	}
	return sorted(sel), nil
}

func sorted(m map[string]Rule) []Rule {
	out := make([]Rule, 0, len(m))
	for _, id := range slices.Sorted(maps.Keys(m)) {
		out = append(out, m[id])
	}
	return out
}
