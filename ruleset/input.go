package ruleset

import (
	"encoding/json"
	"fmt"

	descruntime "ocm.software/open-component-model/bindings/go/descriptor/runtime"
	"ocm.software/open-component-model/bindings/go/runtime"

	"github.com/rebEllieous/ocm-lint-solar/lint"
)

// These types are what CEL expressions see. They are Go structs, not maps, so the
// type checker rejects misspelled fields at load. Pointer fields compare != null
// even when nil, so absence is modelled with zero values and booleans.

type Component struct {
	Name       string      `cel:"name"`
	Version    string      `cel:"version"`
	Labels     []Label     `cel:"labels"`
	Resources  []Resource  `cel:"resources"`
	Sources    []Source    `cel:"sources"`
	References []Reference `cel:"references"`
}

type Resource struct {
	Name            string  `cel:"name"`
	Version         string  `cel:"version"`
	Type            string  `cel:"type"`
	Relation        string  `cel:"relation"`
	Labels          []Label `cel:"labels"`
	HasDigest       bool    `cel:"hasDigest"`
	Access          Access  `cel:"access"`
	EffectiveAccess Access  `cel:"effectiveAccess"`
}

type Source struct {
	Name            string  `cel:"name"`
	Version         string  `cel:"version"`
	Type            string  `cel:"type"`
	Labels          []Label `cel:"labels"`
	Access          Access  `cel:"access"`
	EffectiveAccess Access  `cel:"effectiveAccess"`
}

type Reference struct {
	Name      string  `cel:"name"`
	Component string  `cel:"component"`
	Version   string  `cel:"version"`
	Labels    []Label `cel:"labels"`
}

type Label struct {
	Name  string `cel:"name"`
	Value any    `cel:"value"`
}

// Access is lint.Access for expressions. Undecodable access has Type "" and all
// booleans false.
type Access struct {
	Type            string `cel:"type"`
	ImageReference  string `cel:"imageReference"`
	Reference       string `cel:"reference"`
	MediaType       string `cel:"mediaType"`
	ReferenceName   string `cel:"referenceName"`
	IsOCI           bool   `cel:"isOCI"`
	IsLocalBlob     bool   `cel:"isLocalBlob"`
	IsRelative      bool   `cel:"isRelative"`
	HoldsOCIContent bool   `cel:"holdsOCIContent"`
}

type Location struct {
	Spec      string `cel:"spec"`
	Host      string `cel:"host"`
	Namespace string `cel:"namespace"`
}

type Options struct {
	AllowHosts []string `cel:"allowHosts"`
}

// Problem is one result of a function such as schemaErrors().
type Problem struct {
	Subject string `cel:"subject"`
	Message string `cel:"message"`
}

func newInput(t *lint.Target) (map[string]any, error) {
	c := t.Descriptor.Component
	out := Component{Name: c.Name, Version: c.Version}
	var err error
	if out.Labels, err = labels(c.Labels); err != nil {
		return nil, err
	}
	for _, r := range c.Resources {
		ls, err := labels(r.Labels)
		if err != nil {
			return nil, fmt.Errorf("resource %s: %w", r.Name, err)
		}
		a, eff := accesses(r.Access)
		out.Resources = append(out.Resources, Resource{
			Name: r.Name, Version: r.Version, Type: r.Type, Relation: string(r.Relation), Labels: ls,
			HasDigest: r.Digest != nil, Access: a, EffectiveAccess: eff,
		})
	}
	for _, s := range c.Sources {
		ls, err := labels(s.Labels)
		if err != nil {
			return nil, fmt.Errorf("source %s: %w", s.Name, err)
		}
		a, eff := accesses(s.Access)
		out.Sources = append(out.Sources, Source{Name: s.Name, Version: s.Version, Type: s.Type, Labels: ls, Access: a, EffectiveAccess: eff})
	}
	for _, r := range c.References {
		ls, err := labels(r.Labels)
		if err != nil {
			return nil, fmt.Errorf("reference %s: %w", r.Name, err)
		}
		out.References = append(out.References, Reference{Name: r.Name, Component: r.Component, Version: r.Version, Labels: ls})
	}
	return map[string]any{
		"component": out,
		"location":  Location{Spec: t.Location.Spec, Host: t.Location.Host, Namespace: t.Location.Namespace},
		"options":   Options{AllowHosts: t.Options.AllowHosts},
	}, nil
}

func labels(in []descruntime.Label) ([]Label, error) {
	var out []Label
	for _, l := range in {
		var v any // a label without a value reads as null
		if len(l.Value) > 0 {
			if err := json.Unmarshal(l.Value, &v); err != nil {
				return nil, fmt.Errorf("label %s: %w", l.Name, err)
			}
		}
		out = append(out, Label{Name: l.Name, Value: v})
	}
	return out, nil
}

// accesses returns the access and the effective access: the decoded globalAccess
// when there is one, otherwise the access itself.
func accesses(a runtime.Typed) (access, effective Access) {
	la, err := lint.AccessOf(a)
	if err != nil {
		return Access{}, Access{}
	}
	access = fromLint(la)
	if g, ok := la.Global(); ok {
		return access, fromLint(g)
	}
	return access, access
}

func fromLint(a lint.Access) Access {
	return Access{
		Type: a.Type, ImageReference: a.ImageReference, Reference: a.Reference, MediaType: a.MediaType, ReferenceName: a.ReferenceName,
		IsOCI: a.IsOCI(), IsLocalBlob: a.IsLocalBlob(), IsRelative: a.IsRelative(), HoldsOCIContent: a.HoldsOCIContent(),
	}
}
