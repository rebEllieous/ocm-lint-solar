package lint_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/rebEllieous/ocm-lint-solar/internal/fixture"
	"github.com/rebEllieous/ocm-lint-solar/lint"
)

// names records which components a rule ran on.
func names(seen *[]string) lint.Rule {
	return lint.Func{RuleID: "t/seen", Sev: lint.Warn, Fn: func(_ context.Context, t *lint.Target) ([]lint.Finding, error) {
		*seen = append(*seen, t.Name())
		return nil, nil
	}}
}

func TestRunFillsFindingsAndKeepsGoingAfterToolError(t *testing.T) {
	d := fixture.Descriptor(t, fixture.Component("acme.org/app", "1.0.0", "", ""))
	tg := lint.NewTarget(d, fixture.Repo(), lint.Location{}, lint.Options{})
	broken := lint.Func{RuleID: "t/broken", Fn: func(context.Context, *lint.Target) ([]lint.Finding, error) {
		return nil, errors.New("network down")
	}}
	finds := lint.Func{RuleID: "t/finds", Sev: lint.Error, Fn: func(context.Context, *lint.Target) ([]lint.Finding, error) {
		return []lint.Finding{lint.F("resource b", "x"), lint.F("resource a", "y")}, nil
	}}

	res := lint.Run(context.Background(), tg, []lint.Rule{broken, finds})
	if len(res.Errors) != 1 || res.Errors[0].RuleID != "t/broken" || res.Errors[0].Component != "acme.org/app:1.0.0" {
		t.Fatalf("Errors = %+v", res.Errors)
	}
	if len(res.Findings) != 2 || res.Findings[0].Subject != "resource a" || res.Findings[0].Severity != lint.Error ||
		res.Findings[0].RuleID != "t/finds" || res.Findings[0].Component != "acme.org/app:1.0.0" {
		t.Fatalf("Findings = %+v", res.Findings)
	}
	if !res.HasErrors() {
		t.Fatal("HasErrors = false")
	}
}

func TestLintRecursive(t *testing.T) {
	// app -> a, b; a -> shared; b -> shared, missing; shared -> app (cycle)
	c := fixture.Component
	r := fixture.Ref
	app := fixture.Descriptor(t, c("acme.org/app", "1", "", r("a", "acme.org/a", "1")+","+r("b", "acme.org/b", "1")))
	a := fixture.Descriptor(t, c("acme.org/a", "1", "", r("s", "acme.org/shared", "1")))
	b := fixture.Descriptor(t, c("acme.org/b", "1", "", r("s", "acme.org/shared", "1")+","+r("m", "acme.org/missing", "1")))
	shared := fixture.Descriptor(t, c("acme.org/shared", "1", "", r("app", "acme.org/app", "1")))
	repo := fixture.Repo(app, a, b, shared)

	var seen []string
	root := lint.NewTarget(app, repo, lint.Location{Spec: "ctf::x"}, lint.Options{})
	lint.Lint(context.Background(), root, []lint.Rule{names(&seen)}, true, map[string]bool{})
	want := []string{"acme.org/app:1", "acme.org/a:1", "acme.org/b:1", "acme.org/shared:1"}
	if !slices.Equal(seen, want) {
		t.Fatalf("visited %v, want %v", seen, want)
	}

	seen = nil
	lint.Lint(context.Background(), root, []lint.Rule{names(&seen)}, false, map[string]bool{})
	if !slices.Equal(seen, []string{"acme.org/app:1"}) {
		t.Fatalf("non-recursive visited %v", seen)
	}
}
