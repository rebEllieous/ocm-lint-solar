// Package solar holds rules for components delivered through Solution Arsenal.
//
// These rules mirror Solar code and can drift from it
package solar

import (
	"context"

	descruntime "ocm.software/open-component-model/bindings/go/descriptor/runtime"

	"github.com/rebEllieous/ocm-lint-solar/lint"
)

const (
	helmChartType  = "helmChart"
	ValuesForLabel = "opendefense.cloud/helm/values-for"
)

func init() {
	lint.Register(lint.Func{RuleID: "solar/single-helm-chart", Sev: lint.Error,
		Desc: "exactly one helmChart resource, with OCI access", Fn: singleHelmChart})
	lint.Register(lint.Func{RuleID: "solar/values-template-present", Sev: lint.Error,
		Desc: "exactly one localBlob values template labelled for the chart", Fn: valuesTemplatePresent})
	lint.Register(lint.Func{RuleID: "solar/values-template-only-localblob", Sev: lint.Error,
		Desc: "localBlob is used only by the values template", Fn: onlyTemplateIsLocal})
}

func charts(t *lint.Target) []descruntime.Resource {
	var out []descruntime.Resource
	for _, r := range t.Descriptor.Component.Resources {
		if r.Type == helmChartType {
			out = append(out, r)
		}
	}
	return out
}

func singleHelmChart(_ context.Context, t *lint.Target) ([]lint.Finding, error) {
	cs := charts(t)
	if len(cs) != 1 {
		return []lint.Finding{lint.F("component", "%d helmChart resources, Solar discovery needs exactly one and drops the component otherwise", len(cs))}, nil
	}
	a, err := lint.AccessOf(cs[0].Access)
	if err != nil {
		return []lint.Finding{lint.F("resource "+cs[0].Name, "%v", err)}, nil
	}
	if g, ok := a.Global(); ok {
		a = g
	}
	if !a.IsOCI() {
		return []lint.Finding{lint.F("resource "+cs[0].Name, "chart uses %s access, Solar needs it published as an OCI artifact", a.Type)}, nil
	}
	return nil, nil
}

func valuesTemplatePresent(_ context.Context, t *lint.Target) ([]lint.Finding, error) {
	cs := charts(t)
	if len(cs) != 1 {
		return nil, nil // solar/single-helm-chart reports this
	}
	chart := cs[0].Name
	var tmpls []descruntime.Resource
	for _, r := range t.Descriptor.Component.Resources {
		if v, ok := lint.LabelString(r.Labels, ValuesForLabel); ok && v == chart {
			tmpls = append(tmpls, r)
		}
	}
	switch len(tmpls) {
	case 0:
		return []lint.Finding{lint.F("resource "+chart,
			"no values template (label %s=%q): image references and imagePullSecrets are not relocated, so deployments on air-gapped targets or behind private registries fail to pull",
			ValuesForLabel, chart)}, nil
	case 1:
		if a, err := lint.AccessOf(tmpls[0].Access); err != nil || !a.IsLocalBlob() {
			return []lint.Finding{lint.F("resource "+tmpls[0].Name, "values template must be a localBlob")}, nil
		}
		return nil, nil
	default:
		return []lint.Finding{lint.F("resource "+chart, "%d values templates for this chart, want one", len(tmpls))}, nil
	}
}

func onlyTemplateIsLocal(_ context.Context, t *lint.Target) ([]lint.Finding, error) {
	var fs []lint.Finding
	for _, r := range t.Descriptor.Component.Resources {
		a, err := lint.AccessOf(r.Access)
		if err != nil || !a.IsLocalBlob() {
			continue
		}
		if _, isTemplate := lint.LabelString(r.Labels, ValuesForLabel); !isTemplate {
			fs = append(fs, lint.F("resource "+r.Name, "localBlob access; only the values template may be a localBlob, publish this as an OCI artifact"))
		}
	}
	return fs, nil
}
