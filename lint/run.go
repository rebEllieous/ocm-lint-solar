package lint

import (
	"cmp"
	"context"
	"slices"
)

type Result struct {
	Findings []Finding   `json:"findings"`
	Errors   []ToolError `json:"toolErrors"`
}

func (r *Result) Add(o Result) {
	r.Findings = append(r.Findings, o.Findings...)
	r.Errors = append(r.Errors, o.Errors...)
}

// HasErrors reports an Error-severity finding. Tool errors are checked separately.
func (r Result) HasErrors() bool {
	return slices.ContainsFunc(r.Findings, func(f Finding) bool { return f.Severity == Error })
}

// Run executes the rules in sequence against one target.
func Run(ctx context.Context, t *Target, rules []Rule) Result {
	var res Result
	for _, r := range rules {
		fs, err := r.Check(ctx, t)
		if err != nil {
			res.Errors = append(res.Errors, ToolError{Component: t.Name(), RuleID: r.ID(), Message: err.Error()})
			continue
		}
		for _, f := range fs {
			f.Component, f.RuleID, f.Severity = t.Name(), r.ID(), r.Severity()
			res.Findings = append(res.Findings, f)
		}
	}
	slices.SortStableFunc(res.Findings, func(a, b Finding) int {
		return cmp.Or(cmp.Compare(a.RuleID, b.RuleID), cmp.Compare(a.Subject, b.Subject))
	})
	return res
}

// Lint runs the rules on root and, when recursive, on every component reachable
// through references in root's repository, breadth-first. visited is shared
// across all refs of one invocation so each component is linted once.
// Unresolvable references are skipped here; core/references-resolvable reports them.
func Lint(ctx context.Context, root *Target, rules []Rule, recursive bool, visited map[string]bool) Result {
	var res Result
	queue := []*Target{root}
	for len(queue) > 0 {
		t := queue[0]
		queue = queue[1:]
		if visited[t.Key()] {
			continue
		}
		visited[t.Key()] = true
		res.Add(Run(ctx, t, rules))
		if !recursive {
			continue
		}
		for _, ref := range t.Descriptor.Component.References {
			if d, err := t.Resolve(ctx, ref.Component, ref.Version); err == nil {
				queue = append(queue, t.Child(d))
			}
		}
	}
	return res
}
