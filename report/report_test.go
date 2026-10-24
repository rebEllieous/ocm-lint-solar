package report_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rebEllieous/ocm-lint-solar/lint"
	"github.com/rebEllieous/ocm-lint-solar/report"
)

var res = lint.Result{
	Findings: []lint.Finding{
		{Component: "acme.org/app:1", RuleID: "core/digest-present", Severity: lint.Warn, Subject: "resource a", Message: "no digest"},
		{Component: "acme.org/dep:1", RuleID: "air-gap/no-relative-access", Severity: lint.Error, Subject: "resource b", Message: "relative"},
		{Component: "acme.org/app:1", RuleID: "solar/single-helm-chart", Severity: lint.Error, Subject: "component", Message: "0 charts"},
	},
	Errors: []lint.ToolError{{Component: "acme.org/dep:1", RuleID: "core/references-resolvable", Message: "network down"}},
}

func TestText(t *testing.T) {
	var b bytes.Buffer
	if err := report.Text(&b, res); err != nil {
		t.Fatal(err)
	}
	want := `acme.org/app:1
  warn   core/digest-present  resource a: no digest
  error  solar/single-helm-chart  component: 0 charts
acme.org/dep:1
  error  air-gap/no-relative-access  resource b: relative
tool errors
  acme.org/dep:1  core/references-resolvable: network down
`
	if b.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", b.String(), want)
	}
}

func TestJSON(t *testing.T) {
	var b bytes.Buffer
	if err := report.JSON(&b, res); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Findings []map[string]string `json:"findings"`
		Errors   []map[string]string `json:"toolErrors"`
	}
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Findings) != 3 || got.Findings[1]["severity"] != "error" || got.Findings[1]["rule"] != "air-gap/no-relative-access" ||
		got.Findings[1]["component"] != "acme.org/dep:1" || len(got.Errors) != 1 {
		t.Fatalf("got %+v", got)
	}

	b.Reset()
	_ = report.JSON(&b, lint.Result{})
	if !strings.Contains(b.String(), `"findings":[]`) || !strings.Contains(b.String(), `"toolErrors":[]`) {
		t.Fatalf("empty result = %s", b.String())
	}
}
