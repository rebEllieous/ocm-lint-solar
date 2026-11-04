package ruleset_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rebEllieous/ocm-lint-solar/internal/fixture"
	"github.com/rebEllieous/ocm-lint-solar/lint"
	"github.com/rebEllieous/ocm-lint-solar/ruleset"
)

// file writes a rule file and returns its path.
func file(t *testing.T, yaml string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "rules.yaml")
	if err := os.WriteFile(p, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// spec renders one valid rule, with key/value pairs replacing or, when the value
// is empty, removing a field.
func spec(kv ...string) string {
	fields := map[string]string{"id": "t/r", "severity": "error", "description": "d", "check": "true", "message": `"m"`}
	for i := 0; i < len(kv); i += 2 {
		fields[kv[i]] = kv[i+1]
	}
	var b strings.Builder
	b.WriteString("rules:\n- ")
	for _, k := range []string{"id", "severity", "description", "forEach", "check", "subject", "message"} {
		if v := fields[k]; v != "" {
			fmt.Fprintf(&b, "%s: %q\n  ", k, v)
		}
	}
	return strings.TrimRight(b.String(), " ")
}

func TestLoadErrors(t *testing.T) {
	tests := []struct{ name, yaml string }{
		{"no rules", "rules: []\n"},
		{"empty file", ""},
		{"invalid yaml", "rules: [\n"},
		{"second document", spec() + "---\n" + spec("id", "t/b")},
		{"invalid second document", spec() + "---\nnot: [valid\n"},
		{"unknown field", spec() + "  chek: \"true\"\n"},
		{"missing id", spec("id", "")},
		{"missing severity", spec("severity", "")},
		{"missing description", spec("description", "")},
		{"missing check", spec("check", "")},
		{"missing message", spec("message", "")},
		{"id without pack", spec("id", "nopack")},
		{"id with empty pack", spec("id", "/x")},
		{"id with empty name", spec("id", "core/")},
		{"bad severity", spec("severity", "fatal")},
		{"duplicate id", spec() + "- id: t/r\n  severity: warn\n  description: d\n  check: \"true\"\n  message: '\"m\"'\n"},
		{"check does not parse", spec("check", "1 +")},
		{"check not bool", spec("check", "component.name")},
		{"message not string", spec("message", "component.resources")},
		{"subject not string", spec("subject", "component.resources")},
		{"forEach not a list", spec("forEach", "component.name")},
		{"forEach dyn", spec("forEach", "component.resources[0].labels[0].value")},
		{"item without forEach", spec("check", `item.name == ""`)},
		{"non-resource forEach without subject", spec("forEach", "component.references")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := file(t, tt.yaml)
			_, _, err := ruleset.Load([]string{p})
			if err == nil {
				t.Fatal("no error")
			}
			if !strings.Contains(err.Error(), p) {
				t.Fatalf("error does not name the file: %v", err)
			}
		})
	}
	t.Run("missing path", func(t *testing.T) {
		if _, _, err := ruleset.Load([]string{filepath.Join(t.TempDir(), "nope.yaml")}); err == nil {
			t.Fatal("no error")
		}
	})
}

func TestLoadRejectsMisspelledFields(t *testing.T) {
	const each = "component.resources"
	tests := []struct {
		name string
		kv   []string
	}{
		{"behind or", []string{"check", `component.typo == "x" || true`}},
		{"behind and", []string{"check", `component.typo == "x" && false`}},
		{"in all on empty sources", []string{"check", `component.sources.all(s, s.typo == "x")`}},
		{"in filter then all", []string{"check", `component.resources.filter(r, r.type == "helmChart").all(r, r.typo == "x")`}},
		{"in exists behind or", []string{"check", `component.resources.exists(r, r.typo == "x" || r.name == "a")`}},
		{"on access", []string{"check", `component.resources.exists(r, r.access.typo == "x")`}},
		{"on effectiveAccess", []string{"check", `component.resources.exists(r, r.effectiveAccess.typo == "x")`}},
		{"on label", []string{"check", `component.resources.exists(r, r.labels.exists(l, l.nmae == "x"))`}},
		{"on reference", []string{"check", `component.references.all(r, r.typo == "x")`}},
		{"on bind variable", []string{"check", `cel.bind(cs, component.resources, cs.all(c, c.typo == "x"))`}},
		{"on item in check", []string{"forEach", each, "check", `item.typo == "x"`}},
		{"on item in message", []string{"forEach", each, "message", `item.typo`}},
		{"on item in subject", []string{"forEach", each, "subject", `item.typo`}},
		{"on location", []string{"check", `location.typo == ""`}},
		{"on options", []string{"check", `options.typo == []`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ruleset.Load([]string{file(t, spec(tt.kv...))})
			if err == nil || !strings.Contains(err.Error(), "undefined field") {
				t.Fatalf("err = %v, want undefined field", err)
			}
		})
	}
}

func TestLoadNoPaths(t *testing.T) {
	if _, _, err := ruleset.Load(nil); err == nil {
		t.Fatal("no error")
	}
}

func TestLoadSurroundingDocumentMarkers(t *testing.T) {
	if _, _, err := ruleset.Load([]string{file(t, "---\n"+spec()+"---\n")}); err != nil {
		t.Fatal(err)
	}
}

// YAML reads an unquoted false as a bool; the decoder turns it into the string
// "false", which is CEL for what the author wrote.
func TestUnquotedBoolCheckMeansWhatItSays(t *testing.T) {
	got, err := check(t, "rules:\n- id: t/r\n  severity: error\n  description: d\n  check: false\n  message: '\"m\"'\n", "")
	if err != nil || len(got) != 1 {
		t.Fatalf("findings %+v, err %v", got, err)
	}
}

// check loads one rule and runs it on a component with the given resources.
func check(t *testing.T, yaml, resources string) ([]lint.Finding, error) {
	t.Helper()
	rules, _, err := ruleset.Load([]string{file(t, yaml)})
	if err != nil {
		t.Fatal(err)
	}
	d := fixture.Descriptor(t, fixture.Component("acme.org/app", "1.0.0", resources, ""))
	return rules[0].Check(context.Background(), lint.NewTarget(d, fixture.Repo(), lint.Location{}, lint.Options{}))
}
