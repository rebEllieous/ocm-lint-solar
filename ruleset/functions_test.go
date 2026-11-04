package ruleset_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	descruntime "ocm.software/open-component-model/bindings/go/descriptor/runtime"

	"github.com/rebEllieous/ocm-lint-solar/internal/fixture"
	"github.com/rebEllieous/ocm-lint-solar/lint"
	"github.com/rebEllieous/ocm-lint-solar/ruleset"
)

// problems runs a rule that reports every Problem the function returns.
func problems(t *testing.T, fn string, tg *lint.Target) ([]lint.Finding, error) {
	t.Helper()
	rules, _, err := ruleset.Load([]string{file(t, spec("forEach", fn+"()", "check", "false", "subject", "item.subject", "message", "item.message"))})
	if err != nil {
		t.Fatal(err)
	}
	return rules[0].Check(context.Background(), tg)
}

func TestProblemFunctions(t *testing.T) {
	c, r := fixture.Component, fixture.Ref
	img := fixture.OCIResource("img", "ociImage", "ghcr.io/a/b:1")
	dep := fixture.Descriptor(t, c("acme.org/dep", "1.0.0", "", ""))
	tests := []struct {
		name, fn, js    string
		subject, inText string // empty means no finding
	}{
		{"schema valid", "schemaErrors", c("acme.org/app", "1.0.0", img, ""), "", ""},
		{"schema invalid", "schemaErrors", c("NOT A VALID NAME", "1.0.0", img, ""), "descriptor", "name"},
		{"identity unique", "identityErrors", c("acme.org/app", "1.0.0", img, ""), "", ""},
		{"identity duplicate", "identityErrors", c("acme.org/app", "1.0.0", img+","+img, ""), "descriptor", "img"},
		{"reference present", "unresolvedReferences", c("acme.org/app", "1.0.0", "", r("dep", "acme.org/dep", "1.0.0")), "", ""},
		{"reference missing", "unresolvedReferences", c("acme.org/app", "1.0.0", "", r("dep", "acme.org/gone", "1.0.0")), "reference dep (acme.org/gone:1.0.0)", "not found in"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tg := lint.NewTarget(fixture.Descriptor(t, tt.js), fixture.Repo(dep), lint.Location{}, lint.Options{})
			got, err := problems(t, tt.fn, tg)
			if err != nil {
				t.Fatal(err)
			}
			if tt.subject == "" {
				if len(got) != 0 {
					t.Fatalf("findings: %+v", got)
				}
				return
			}
			if len(got) != 1 || got[0].Subject != tt.subject || !strings.Contains(got[0].Message, tt.inText) {
				t.Fatalf("findings: %+v", got)
			}
		})
	}
}

func TestUnresolvedTransitiveReferences(t *testing.T) {
	c, r := fixture.Component, fixture.Ref
	// app -> a -> b -> missing, b -> app (cycle); app -> gone (direct, not reported here)
	app := fixture.Descriptor(t, c("acme.org/app", "1", "", r("a", "acme.org/a", "1")+","+r("g", "acme.org/gone", "1")))
	a := fixture.Descriptor(t, c("acme.org/a", "1", "", r("b", "acme.org/b", "1")))
	b := fixture.Descriptor(t, c("acme.org/b", "1", "", r("m", "acme.org/missing", "1")+","+r("app", "acme.org/app", "1")))
	tg := lint.NewTarget(app, fixture.Repo(app, a, b), lint.Location{Spec: "reg"}, lint.Options{})

	got, err := problems(t, "unresolvedTransitiveReferences", tg)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Subject != "reference acme.org/app:1 -> acme.org/a:1 -> acme.org/b:1 -> acme.org/missing:1" {
		t.Fatalf("findings = %+v", got)
	}
}

type downRepo struct{}

func (downRepo) GetComponentVersion(context.Context, string, string) (*descruntime.Descriptor, error) {
	return nil, errors.New("registry down")
}

func TestFunctionRegistryFailureIsAToolError(t *testing.T) {
	d := fixture.Descriptor(t, fixture.Component("acme.org/app", "1", "", fixture.Ref("dep", "acme.org/dep", "1")))
	for _, fn := range []string{"unresolvedReferences", "unresolvedTransitiveReferences"} {
		t.Run(fn, func(t *testing.T) {
			got, err := problems(t, fn, lint.NewTarget(d, downRepo{}, lint.Location{}, lint.Options{}))
			if err == nil || !strings.Contains(err.Error(), "registry down") || got != nil {
				t.Fatalf("findings %+v, err %v", got, err)
			}
		})
	}
}

func TestExternalReason(t *testing.T) {
	loc := lint.Location{Spec: "reg.local:5000/catalog", Host: "reg.local:5000", Namespace: "catalog"}
	ctf := lint.Location{Spec: "ctf::x"}
	tests := []struct {
		name, ref string
		loc       lint.Location
		allow     []string
		external  bool
	}{
		{"same host", "reg.local:5000/other/app:1", loc, nil, false},
		{"reanchored path", "localhost:4443/catalog/app:1", loc, nil, false},
		{"external", "ghcr.io/acme/app:1", loc, nil, true},
		{"allowed", "ghcr.io/acme/app:1", loc, []string{"ghcr.io"}, false},
		{"ctf has no host", "reg.local:5000/catalog/app:1", ctf, nil, true},
		{"unparsable", "UPPER CASE::bad", loc, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules, _, err := ruleset.Load([]string{file(t, spec("check", `externalReason("`+tt.ref+`") == ""`, "message", `externalReason("`+tt.ref+`")`))})
			if err != nil {
				t.Fatal(err)
			}
			d := fixture.Descriptor(t, fixture.Component("acme.org/app", "1", "", ""))
			got, err := rules[0].Check(context.Background(), lint.NewTarget(d, fixture.Repo(), tt.loc, lint.Options{AllowHosts: tt.allow}))
			if err != nil {
				t.Fatal(err)
			}
			if (len(got) == 1) != tt.external {
				t.Fatalf("findings = %+v, external %v", got, tt.external)
			}
			if tt.external && got[0].Message == "" {
				t.Fatal("empty reason")
			}
		})
	}
}
