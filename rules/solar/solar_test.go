package solar_test

import (
	"context"
	"strings"
	"testing"

	"github.com/rebEllieous/ocm-lint-solar/internal/fixture"
	"github.com/rebEllieous/ocm-lint-solar/lint"
	_ "github.com/rebEllieous/ocm-lint-solar/rules/solar"
)

func run(t *testing.T, id, resources string) []lint.Finding {
	t.Helper()
	d := fixture.Descriptor(t, fixture.Component("acme.org/app", "1.0.0", resources, ""))
	tg := lint.NewTarget(d, fixture.Repo(), lint.Location{}, lint.Options{})
	for _, r := range lint.Default.All() {
		if r.ID() == id {
			f, err := r.Check(context.Background(), tg)
			if err != nil {
				t.Fatal(err)
			}
			return f
		}
	}
	t.Fatalf("rule %s not registered", id)
	return nil
}

func TestSolar(t *testing.T) {
	chart := fixture.OCIResource("chart", "helmChart", "reg.local/catalog/chart:1")
	chart2 := fixture.OCIResource("chart2", "helmChart", "reg.local/catalog/chart2:1")
	blobChart := fixture.LocalBlobResource("chart", "helmChart", "application/vnd.oci.image.manifest.v1+tar+gzip", "acme/chart", "")
	label := func(v string) string { return `{"name":"opendefense.cloud/helm/values-for","value":` + v + `}` }
	tmpl := fixture.LocalBlobResource("values", "yaml", "application/x-yaml", "", label(`"chart"`))
	tmpl2 := fixture.LocalBlobResource("values2", "yaml", "application/x-yaml", "", label(`"chart"`))
	tmplOther := fixture.LocalBlobResource("values", "yaml", "application/x-yaml", "", label(`"nope"`))
	tmplNotString := fixture.LocalBlobResource("values", "yaml", "application/x-yaml", "", label(`{"x":1}`))
	tmplOCI := `{"name":"values","version":"1.0.0","type":"yaml","relation":"external","labels":[` + label(`"chart"`) + `],"access":{"type":"OCIImage","imageReference":"reg.local/catalog/values:1"}}`
	strayBlob := fixture.LocalBlobResource("notes", "plainText", "text/plain", "", "")
	j := func(rs ...string) string { return strings.Join(rs, ",") }

	tests := []struct {
		name, rule, res string
		count           int
	}{
		{"one OCI chart", "solar/single-helm-chart", j(chart, tmpl), 0},
		{"no chart", "solar/single-helm-chart", tmpl, 1},
		{"two charts", "solar/single-helm-chart", j(chart, chart2), 1},
		{"chart as local blob", "solar/single-helm-chart", blobChart, 1},
		{"template present", "solar/values-template-present", j(chart, tmpl), 0},
		{"template missing", "solar/values-template-present", chart, 1},
		{"template for other chart", "solar/values-template-present", j(chart, tmplOther), 1},
		{"template label not a string", "solar/values-template-present", j(chart, tmplNotString), 1},
		{"two templates", "solar/values-template-present", j(chart, tmpl, tmpl2), 1},
		{"template not a local blob", "solar/values-template-present", j(chart, tmplOCI), 1},
		{"no chart: leave it to single-helm-chart", "solar/values-template-present", tmpl, 0},
		{"only template is local", "solar/values-template-only-localblob", j(chart, tmpl), 0},
		{"stray local blob", "solar/values-template-only-localblob", j(chart, tmpl, strayBlob), 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := run(t, tt.rule, tt.res); len(got) != tt.count {
				t.Fatalf("%d findings, want %d: %+v", len(got), tt.count, got)
			}
		})
	}
}

func TestMissingTemplateMessageNamesTheConsequence(t *testing.T) {
	got := run(t, "solar/values-template-present", fixture.OCIResource("chart", "helmChart", "reg.local/catalog/chart:1"))
	if len(got) != 1 || !strings.Contains(got[0].Message, "air-gapped") {
		t.Fatalf("findings = %+v", got)
	}
}
