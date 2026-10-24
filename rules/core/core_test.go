package core_test

import (
	"context"
	"testing"

	"github.com/rebEllieous/ocm-lint-solar/internal/fixture"
	"github.com/rebEllieous/ocm-lint-solar/lint"
	_ "github.com/rebEllieous/ocm-lint-solar/rules/core"
)

func check(t *testing.T, id string, tg *lint.Target) []lint.Finding {
	t.Helper()
	rules, err := lint.Default.Select([]string{id}, nil)
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
	t.Fatalf("rule %s not registered", id)
	return nil
}

func target(t *testing.T, js string, repo lint.Getter) *lint.Target {
	return lint.NewTarget(fixture.Descriptor(t, js), repo, lint.Location{}, lint.Options{})
}

const digest = `,"digest":{"hashAlgorithm":"SHA-256","normalisationAlgorithm":"genericBlobDigest/v1","value":"00"}`

func TestCore(t *testing.T) {
	c := fixture.Component
	ok := fixture.OCIResource("img", "ociImage", "ghcr.io/a/b:1")
	okDigest := ok[:len(ok)-1] + digest + "}"
	dep := fixture.Descriptor(t, c("acme.org/dep", "1.0.0", "", ""))
	repo := fixture.Repo(dep)

	tests := []struct {
		rule  string
		js    string
		count int
	}{
		{"core/schema-valid", c("acme.org/app", "1.0.0", okDigest, ""), 0},
		{"core/schema-valid", c("NOT A VALID NAME", "1.0.0", okDigest, ""), 1},
		{"core/unique-identity", c("acme.org/app", "1.0.0", ok, ""), 0},
		{"core/unique-identity", c("acme.org/app", "1.0.0", ok+","+ok, ""), 1},
		{"core/references-resolvable", c("acme.org/app", "1.0.0", "", fixture.Ref("dep", "acme.org/dep", "1.0.0")), 0},
		{"core/references-resolvable", c("acme.org/app", "1.0.0", "", fixture.Ref("dep", "acme.org/gone", "1.0.0")), 1},
		{"core/digest-present", c("acme.org/app", "1.0.0", okDigest, ""), 0},
		{"core/digest-present", c("acme.org/app", "1.0.0", ok, ""), 1},
	}
	for _, tt := range tests {
		t.Run(tt.rule, func(t *testing.T) {
			if got := check(t, tt.rule, target(t, tt.js, repo)); len(got) != tt.count {
				t.Fatalf("%d findings, want %d: %+v", len(got), tt.count, got)
			}
		})
	}
}
