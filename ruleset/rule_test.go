package ruleset_test

import (
	"context"
	"strings"
	"testing"

	"github.com/rebEllieous/ocm-lint-solar/internal/fixture"
	"github.com/rebEllieous/ocm-lint-solar/lint"
	"github.com/rebEllieous/ocm-lint-solar/ruleset"
)

const digest = `,"digest":{"hashAlgorithm":"SHA-256","normalisationAlgorithm":"genericBlobDigest/v1","value":"00"}`

func TestCheckFindings(t *testing.T) {
	a := fixture.OCIResource("a", "ociImage", "ghcr.io/a/a:1")
	b := fixture.OCIResource("b", "helmChart", "ghcr.io/a/b:1")
	const each = "component.resources"
	tests := []struct {
		name      string
		yaml, res string
		want      []lint.Finding
	}{
		{"pass", spec(), a, nil},
		{"component finding", spec("check", "false"), a, []lint.Finding{{Subject: "component", Message: "m"}}},
		{"forEach finds the failing element", spec("forEach", each, "check", `item.type != "helmChart"`), a + "," + b, []lint.Finding{{Subject: "resource b", Message: "m"}}},
		{"forEach over nothing", spec("forEach", each, "check", "false"), "", nil},
		{"subject overrides", spec("check", "false", "subject", `"s-" + component.name`), a, []lint.Finding{{Subject: "s-acme.org/app", Message: "m"}}},
		{"message expression sees item", spec("forEach", each, "check", "false", "message", `item.name + "!"`), a, []lint.Finding{{Subject: "resource a", Message: "a!"}}},
		{"message is not reformatted", spec("check", "false", "message", `"100%d of %s"`), a, []lint.Finding{{Subject: "component", Message: "100%d of %s"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := check(t, tt.yaml, tt.res)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got %+v, want %+v", got, tt.want)
				}
			}
		})
	}
}

func TestRuleMetadata(t *testing.T) {
	rules, _, err := ruleset.Load(context.Background(), []string{file(t, spec("id", "acme/x", "severity", "warn", "description", "the rule"))})
	if err != nil {
		t.Fatal(err)
	}
	r := rules[0]
	if r.ID() != "acme/x" || r.Severity() != lint.Warn || r.Description() != "the rule" {
		t.Fatalf("got %s %s %s", r.ID(), r.Severity(), r.Description())
	}
}

// Each expression must be true on the resource for the rule to pass.
func TestInputs(t *testing.T) {
	withLabels := func(vs ...string) string {
		for i, v := range vs {
			vs[i] = `{"name":"l` + string(rune('a'+i)) + `","value":` + v + `}`
		}
		return `{"name":"r","version":"1.0.0","type":"blob","relation":"external","labels":[` + strings.Join(vs, ",") + `],"access":{"type":"OCIImage/v1","imageReference":"ghcr.io/a/b:1"}}`
	}
	img := fixture.OCIResource("r", "ociImage", "ghcr.io/a/b:1")
	blob := fixture.LocalBlobResource("r", "helmChart", "application/vnd.oci.image.manifest.v1+tar+gzip", "acme/c", "")
	global := `{"name":"r","version":"1.0.0","type":"ociImage","relation":"local","access":{"type":"localBlob","localReference":"sha256:00","mediaType":"application/octet-stream","globalAccess":{"type":"ociArtifact","imageReference":"ghcr.io/a/g:1"}}}`
	const first = "component.resources[0]"

	tests := []struct{ name, expr, res string }{
		{"resource fields", first + `.name == "r" && ` + first + `.version == "1.0.0" && ` + first + `.type == "ociImage" && ` + first + `.relation == "external"`, img},
		{"component fields", `component.name == "acme.org/app" && component.version == "1.0.0" && component.sources.size() == 0 && component.references.size() == 0`, img},
		{"access type drops version", first + `.access.type == "OCIImage" && ` + first + `.access.isOCI && !` + first + `.access.isLocalBlob`, withLabels()},
		{"image reference", first + `.access.imageReference == "ghcr.io/a/b:1"`, img},
		{"local blob", first + `.access.isLocalBlob && ` + first + `.access.holdsOCIContent && ` + first + `.access.referenceName == "acme/c"`, blob},
		{"effective is global", first + `.effectiveAccess.imageReference == "ghcr.io/a/g:1" && ` + first + `.access.isLocalBlob`, global},
		{"effective without global is access", first + `.effectiveAccess.imageReference == ` + first + `.access.imageReference`, img},
		{"no digest", `!` + first + `.hasDigest`, img},
		{"digest", first + `.hasDigest`, img[:len(img)-1] + digest + "}"},
		{"string label", first + `.labels.exists(l, l.value == "chart")`, withLabels(`"chart"`)},
		{"non-string labels compare false", `!` + first + `.labels.exists(l, l.value == "chart")`, withLabels(`null`, `{"x":1}`, `["a"]`, `1`, `true`)},
		{"string test", first + `.labels.exists(l, type(l.value) == string)`, withLabels(`"chart"`)},
		{"string test on non-strings", `!` + first + `.labels.exists(l, type(l.value) == string)`, withLabels(`null`, `{"x":1}`)},
		{"location and options", `location.spec == "" && location.host == "" && location.namespace == "" && options.allowHosts.size() == 0`, img},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := check(t, spec("check", tt.expr), tt.res)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 0 {
				t.Fatalf("expression was false: %s", tt.expr)
			}
		})
	}
}

func TestRuntimeErrorsAreToolErrors(t *testing.T) {
	res := `{"name":"r","version":"1.0.0","type":"blob","relation":"external","labels":[{"name":"l","value":"s"}],"access":{"type":"OCIImage/v1","imageReference":"ghcr.io/a/b:1"}}`
	tests := []struct {
		name string
		kv   []string
	}{
		{"in check", []string{"check", `component.resources[5].name == ""`}},
		{"in message", []string{"check", "false", "message", `component.resources[5].name`}},
		{"in subject", []string{"check", "false", "subject", `component.resources[5].name`}},
		{"in forEach", []string{"forEach", "component.resources[5].labels", "subject", "item.name"}},
		{"dyn check is not a bool", []string{"check", `component.resources[0].labels[0].value`}},
		{"dyn message is not a string", []string{"check", "false", "message", `component.resources[0].labels[0].value == "s" ? dyn(1) : dyn("x")`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := check(t, spec(tt.kv...), res)
			if err == nil {
				t.Fatal("no error")
			}
			if got != nil {
				t.Fatalf("findings alongside error: %+v", got)
			}
		})
	}
}

func TestLabelWithoutValue(t *testing.T) {
	res := `{"name":"r","version":"1.0.0","type":"blob","relation":"external","labels":[{"name":"x"}],"access":{"type":"OCIImage/v1","imageReference":"ghcr.io/a/b:1"}}`
	got, err := check(t, spec("check", `!component.resources[0].labels.exists(l, l.value == "chart")`), res)
	if err != nil || len(got) != 0 {
		t.Fatalf("findings %+v, err %v", got, err)
	}
}

func TestFindingsDroppedWhenALaterElementErrors(t *testing.T) {
	a, b := fixture.OCIResource("a", "ociImage", "ghcr.io/a/a:1"), fixture.OCIResource("b", "ociImage", "ghcr.io/a/b:1")
	// a fails normally; b reads past the end of the list while failing.
	got, err := check(t, spec("forEach", "component.resources", "check", "false",
		"message", `item.name == "a" ? "m" : component.resources[5].name`), a+","+b)
	if err == nil || got != nil {
		t.Fatalf("findings %+v, err %v", got, err)
	}
}
