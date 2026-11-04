package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rebEllieous/ocm-lint-solar/internal/fixture"
)

const (
	digest = `,"digest":{"hashAlgorithm":"SHA-256","normalisationAlgorithm":"genericBlobDigest/v1","value":"00"}`

	coreRules   = "../../rules/core.yaml"
	airGapRules = "../../rules/air-gap.yaml"
	solarRules  = "../../rules/solar.yaml"
)

func withDigest(res string) string { return res[:len(res)-1] + digest + "}" }

// ruleFile writes a rule file with one rule and returns its path.
func ruleFile(t *testing.T, id, description, check string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "rules.yaml")
	yaml := "rules:\n- id: " + id + "\n  severity: error\n  description: " + description + "\n  check: '" + check + "'\n  message: '\"m\"'\n"
	if err := os.WriteFile(p, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestExitCodes(t *testing.T) {
	clean := fixture.Descriptor(t, fixture.Component("acme.org/clean", "1.0.0",
		withDigest(fixture.OCIResource("img", "ociImage", "ghcr.io/a/b:1")), ""))
	warnOnly := fixture.Descriptor(t, fixture.Component("acme.org/warn", "1.0.0",
		fixture.OCIResource("img", "ociImage", "ghcr.io/a/b:1"), "")) // no digest: warning
	broken := fixture.Descriptor(t, fixture.Component("acme.org/broken", "1.0.0", "",
		fixture.Ref("dep", "acme.org/gone", "1.0.0"))) // unresolvable: error
	twoCharts := fixture.Descriptor(t, fixture.Component("acme.org/charts", "1.0.0",
		fixture.OCIResource("a", "helmChart", "ghcr.io/a/a:1")+","+fixture.OCIResource("b", "helmChart", "ghcr.io/a/b:1"), ""))
	repo := fixture.Registry(t, clean, warnOnly, broken, twoCharts)
	cleanRef := repo + "//acme.org/clean:1.0.0"

	allow := ruleFile(t, "solar/single-helm-chart", "up to three charts", `component.resources.filter(r, r.type == "helmChart").size() <= 3`)

	tests := []struct {
		name string
		args []string
		want int
	}{
		{"clean", []string{"--rule", coreRules, cleanRef}, 0},
		{"warnings only", []string{"--rule", coreRules, repo + "//acme.org/warn:1.0.0"}, 0},
		{"error finding", []string{"--rule", coreRules, repo + "//acme.org/broken:1.0.0"}, 1},
		{"air-gap finds external image", []string{"--rule", coreRules, "--rule", airGapRules, cleanRef}, 1},
		{"allow-host clears it", []string{"--rule", coreRules, "--rule", airGapRules, "--allow-host", "ghcr.io", cleanRef}, 0},
		{"skip clears a finding", []string{"--rule", coreRules, "--skip", "core/references-resolvable", repo + "//acme.org/broken:1.0.0"}, 0},
		{"no rule", []string{cleanRef}, 2},
		{"missing rule file", []string{"--rule", "nope.yaml", cleanRef}, 2},
		{"pack name is not a file", []string{"--rule", "core", cleanRef}, 2},
		{"skip removes every rule", []string{"--rule", ruleFile(t, "t/a", "d", "true"), "--skip", "t/a", cleanRef}, 2},
		{"unknown skip", []string{"--rule", coreRules, "--skip", "core/nope", cleanRef}, 2},
		{"misspelled field", []string{"--rule", ruleFile(t, "t/a", "d", `component.typo == "x" || true`), cleanRef}, 2},
		{"runtime error in a rule", []string{"--rule", ruleFile(t, "t/a", "d", `component.resources[5].name == ""`), cleanRef}, 2},
		{"baseline rejects two charts", []string{"--rule", solarRules, repo + "//acme.org/charts:1.0.0"}, 1},
		{"override allows two charts", []string{"--rule", solarRules, "--rule", allow, repo + "//acme.org/charts:1.0.0"}, 0},
		{"missing component", []string{"--rule", coreRules, repo + "//acme.org/missing:1.0.0"}, 2},
		{"bad ref", []string{"--rule", coreRules, "not-a-ref"}, 2},
		{"no args", nil, 2},
		{"tool error beats error finding", []string{"--rule", coreRules, repo + "//acme.org/broken:1.0.0", repo + "//acme.org/missing:1.0.0"}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if got := run(context.Background(), tt.args, &out, &errOut); got != tt.want {
				t.Fatalf("exit %d, want %d\nstdout:\n%s\nstderr:\n%s", got, tt.want, out.String(), errOut.String())
			}
		})
	}
}

func TestRecursiveJSON(t *testing.T) {
	dep := fixture.Descriptor(t, fixture.Component("acme.org/dep", "1.0.0", fixture.OCIResource("img", "ociImage", "ghcr.io/a/b:1"), ""))
	app := fixture.Descriptor(t, fixture.Component("acme.org/app", "1.0.0", "", fixture.Ref("dep", "acme.org/dep", "1.0.0")))
	repo := fixture.Registry(t, dep, app)

	var out bytes.Buffer
	run(context.Background(), []string{"--rule", coreRules, "--recursive", "-o", "json", repo + "//acme.org/app:1.0.0"}, &out, &bytes.Buffer{})
	var got struct {
		Findings []struct {
			Component, Rule string
		} `json:"findings"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if len(got.Findings) != 1 || got.Findings[0].Component != "acme.org/dep:1.0.0" || got.Findings[0].Rule != "core/digest-present" {
		t.Fatalf("findings = %+v", got.Findings)
	}
}

func TestListRules(t *testing.T) {
	var out bytes.Buffer
	args := []string{"--rule", coreRules, "--rule", airGapRules, "--rule", solarRules, "--list-rules"}
	if code := run(context.Background(), args, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	for _, id := range []string{"core/schema-valid", "air-gap/references-closed", "solar/values-template-present"} {
		if !strings.Contains(out.String(), id) {
			t.Fatalf("--list-rules misses %s:\n%s", id, out.String())
		}
	}
	if n := strings.Count(out.String(), "\n"); n != 11 {
		t.Fatalf("%d rules listed, want 11:\n%s", n, out.String())
	}
}

func TestListRulesNeedsRuleFiles(t *testing.T) {
	if code := run(context.Background(), []string{"--list-rules"}, &bytes.Buffer{}, &bytes.Buffer{}); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

func TestListRulesShowsOverride(t *testing.T) {
	var out bytes.Buffer
	args := []string{"--rule", solarRules, "--rule", ruleFile(t, "solar/single-helm-chart", "up to three charts", "true"), "--list-rules"}
	if code := run(context.Background(), args, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out.String(), "up to three charts") || strings.Contains(out.String(), "exactly one helmChart") {
		t.Fatalf("override not shown:\n%s", out.String())
	}
}

func TestWarnings(t *testing.T) {
	clean := fixture.Descriptor(t, fixture.Component("acme.org/clean", "1.0.0",
		withDigest(fixture.OCIResource("img", "ociImage", "ghcr.io/a/b:1")), ""))
	ref := fixture.Registry(t, clean) + "//acme.org/clean:1.0.0"
	typo := ruleFile(t, "core/digest-presnt", "d", "true")
	own := ruleFile(t, "acme/a", "d", "true")

	const refsRule = "core/references-resolvable"
	tests := []struct {
		name       string
		args       []string
		contains   string
		notContain string
		want       int
	}{
		{"override typo", []string{"--rule", coreRules, "--rule", typo, ref}, "core/digest-presnt", "", 0},
		{"references rule not loaded", []string{"--rule", own, ref}, refsRule, "", 0},
		{"references rule skipped on purpose", []string{"--rule", coreRules, "--skip", refsRule, ref}, "", refsRule, 0},
		{"core loaded", []string{"--rule", coreRules, ref}, "", refsRule, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			args := append([]string{"-o", "json"}, tt.args...)
			if got := run(context.Background(), args, &out, &errOut); got != tt.want {
				t.Fatalf("exit %d, want %d\nstderr:\n%s", got, tt.want, errOut.String())
			}
			if !json.Valid(out.Bytes()) {
				t.Fatalf("stdout is not JSON:\n%s", out.String())
			}
			if !strings.Contains(errOut.String(), tt.contains) {
				t.Fatalf("stderr misses %q:\n%s", tt.contains, errOut.String())
			}
			if tt.notContain != "" && strings.Contains(errOut.String(), tt.notContain) {
				t.Fatalf("stderr has %q:\n%s", tt.notContain, errOut.String())
			}
		})
	}
}
