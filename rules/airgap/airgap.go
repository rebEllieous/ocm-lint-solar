// Package airgap holds rules for components that must work after a transfer
// into an isolated registry.
package airgap

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	"ocm.software/open-component-model/bindings/go/repository"

	"github.com/rebEllieous/ocm-lint-solar/lint"
)

func init() {
	lint.Register(lint.Func{RuleID: "air-gap/no-relative-access", Sev: lint.Error,
		Desc: "no relativeOciReference access", Fn: noRelativeAccess})
	lint.Register(lint.Func{RuleID: "air-gap/no-external-access", Sev: lint.Error,
		Desc: "OCI accesses point into the repository or an --allow-host", Fn: noExternalAccess})
	lint.Register(lint.Func{RuleID: "air-gap/reference-name-set", Sev: lint.Error,
		Desc: "local blobs holding OCI content have a referenceName", Fn: referenceNameSet})
	lint.Register(lint.Func{RuleID: "air-gap/references-closed", Sev: lint.Error,
		Desc: "indirect component references resolve within the repository", Fn: referencesClosed})
}

func noRelativeAccess(_ context.Context, t *lint.Target) ([]lint.Finding, error) {
	var fs []lint.Finding
	for _, res := range t.Descriptor.Component.Resources {
		if a, err := lint.AccessOf(res.Access); err == nil && a.IsRelative() {
			fs = append(fs, lint.F("resource "+res.Name, "relativeOciReference %q is not supported by OCM v2 tooling and breaks after re-anchoring", a.Reference))
		}
	}
	return fs, nil
}

func noExternalAccess(_ context.Context, t *lint.Target) ([]lint.Finding, error) {
	var fs []lint.Finding
	for _, res := range t.Descriptor.Component.Resources {
		a, err := lint.AccessOf(res.Access)
		if err != nil {
			continue
		}
		if g, ok := a.Global(); ok {
			a = g
		}
		var msg string
		switch {
		case a.IsOCI() || a.Type == "ociBlob":
			msg = external(a.ImageReference, t)
		case a.IsLocalBlob(), a.IsRelative(): // inside the component; relative has its own rule
		default: // helm, wget, s3, github, npm, ...
			msg = fmt.Sprintf("access type %s fetches from outside the repository", a.Type)
		}
		if msg != "" {
			fs = append(fs, lint.F("resource "+res.Name, "%s", msg))
		}
	}
	return fs, nil
}

// external returns why ref is outside the repository, or "" when it is inside.
func external(ref string, t *lint.Target) string {
	parsed, err := name.ParseReference(ref)
	if err != nil {
		return fmt.Sprintf("cannot parse image reference %q: %v", ref, err)
	}
	host, path := parsed.Context().RegistryStr(), parsed.Context().RepositoryStr()
	loc := t.Location
	if slices.Contains(t.Options.AllowHosts, host) {
		return ""
	}
	if loc.Host != "" && host == loc.Host {
		return ""
	}
	// Same re-anchoring rule as Solar: a path under our namespace was copied here,
	// whatever host the transfer was pushed through.
	if loc.Namespace != "" && (path == loc.Namespace || strings.HasPrefix(path, loc.Namespace+"/")) {
		return ""
	}
	return fmt.Sprintf("%s points outside the repository (host %s)", ref, host)
}

func referenceNameSet(_ context.Context, t *lint.Target) ([]lint.Finding, error) {
	var fs []lint.Finding
	for _, res := range t.Descriptor.Component.Resources {
		if a, err := lint.AccessOf(res.Access); err == nil && a.HoldsOCIContent() && a.ReferenceName == "" {
			fs = append(fs, lint.F("resource "+res.Name, "local blob with OCI content (%s) has no referenceName, so a transfer cannot publish it", a.MediaType))
		}
	}
	return fs, nil
}

// referencesClosed walks the reference graph and reports unresolvable references
// at depth 2 or deeper; direct ones are core/references-resolvable.
func referencesClosed(ctx context.Context, t *lint.Target) ([]lint.Finding, error) {
	type node struct {
		d    *lint.Target
		path []string
	}
	var fs []lint.Finding
	seen := map[string]bool{t.Name(): true}
	queue := []node{{t, []string{t.Name()}}}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, ref := range n.d.Descriptor.Component.References {
			key := ref.Component + ":" + ref.Version
			path := append(slices.Clone(n.path), key)
			d, err := t.Resolve(ctx, ref.Component, ref.Version)
			switch {
			case errors.Is(err, repository.ErrNotFound):
				if len(n.path) > 1 {
					fs = append(fs, lint.F("reference "+strings.Join(path, " -> "), "not found in %s", t.Location.Spec))
				}
			case err != nil:
				return nil, fmt.Errorf("resolve %s: %w", key, err)
			case !seen[key]:
				seen[key] = true
				queue = append(queue, node{t.Child(d), path})
			}
		}
	}
	return fs, nil
}
