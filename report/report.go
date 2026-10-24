// Package report renders lint results.
package report

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/rebEllieous/ocm-lint-solar/lint"
)

// Text groups findings by component in first-seen order, then lists tool errors.
func Text(w io.Writer, r lint.Result) error {
	var order []string
	byComp := map[string][]lint.Finding{}
	for _, f := range r.Findings {
		if _, ok := byComp[f.Component]; !ok {
			order = append(order, f.Component)
		}
		byComp[f.Component] = append(byComp[f.Component], f)
	}
	for _, c := range order {
		fmt.Fprintln(w, c)
		for _, f := range byComp[c] {
			fmt.Fprintf(w, "  %-5s  %s  %s: %s\n", f.Severity, f.RuleID, f.Subject, f.Message)
		}
	}
	if len(r.Errors) > 0 {
		fmt.Fprintln(w, "tool errors")
		for _, e := range r.Errors {
			fmt.Fprintf(w, "  %s  %s: %s\n", e.Component, e.RuleID, e.Message)
		}
	}
	return nil
}

func JSON(w io.Writer, r lint.Result) error {
	if r.Findings == nil {
		r.Findings = []lint.Finding{}
	}
	if r.Errors == nil {
		r.Errors = []lint.ToolError{}
	}
	return json.NewEncoder(w).Encode(r)
}
