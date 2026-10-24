// Package core holds generic OCM rules. It is the default pack.
package core

import (
	"context"
	"errors"
	"fmt"

	descruntime "ocm.software/open-component-model/bindings/go/descriptor/runtime"
	descv2 "ocm.software/open-component-model/bindings/go/descriptor/v2"
	"ocm.software/open-component-model/bindings/go/repository"

	"github.com/rebEllieous/ocm-lint-solar/lint"
)

func init() {
	lint.Register(lint.Func{RuleID: "core/schema-valid", Sev: lint.Error,
		Desc: "descriptor validates against the OCM v2 schema", Fn: schemaValid})
	lint.Register(lint.Func{RuleID: "core/unique-identity", Sev: lint.Error,
		Desc: "no duplicate resource, source or reference identity", Fn: uniqueIdentity})
	lint.Register(lint.Func{RuleID: "core/references-resolvable", Sev: lint.Error,
		Desc: "every component reference exists in the repository", Fn: referencesResolvable})
	lint.Register(lint.Func{RuleID: "core/digest-present", Sev: lint.Warn,
		Desc: "every resource has a digest", Fn: digestPresent})
}

func schemaValid(_ context.Context, t *lint.Target) ([]lint.Finding, error) {
	v2, err := descruntime.ConvertToV2(descv2.Scheme, t.Descriptor)
	if err != nil {
		return []lint.Finding{lint.F("descriptor", "cannot convert to v2: %v", err)}, nil
	}
	if err := descv2.Validate(v2); err != nil {
		return []lint.Finding{lint.F("descriptor", "%v", err)}, nil
	}
	return nil, nil
}

func uniqueIdentity(_ context.Context, t *lint.Target) ([]lint.Finding, error) {
	if err := descruntime.Validate(t.Descriptor); err != nil {
		return []lint.Finding{lint.F("descriptor", "%v", err)}, nil
	}
	return nil, nil
}

func referencesResolvable(ctx context.Context, t *lint.Target) ([]lint.Finding, error) {
	var fs []lint.Finding
	for _, ref := range t.Descriptor.Component.References {
		_, err := t.Resolve(ctx, ref.Component, ref.Version)
		switch {
		case errors.Is(err, repository.ErrNotFound):
			fs = append(fs, lint.F(subject(ref), "%s:%s not found in %s", ref.Component, ref.Version, t.Location.Spec))
		case err != nil:
			return nil, fmt.Errorf("resolve %s:%s: %w", ref.Component, ref.Version, err)
		}
	}
	return fs, nil
}

func digestPresent(_ context.Context, t *lint.Target) ([]lint.Finding, error) {
	var fs []lint.Finding
	for _, res := range t.Descriptor.Component.Resources {
		if res.Digest == nil {
			fs = append(fs, lint.F("resource "+res.Name, "no digest"))
		}
	}
	return fs, nil
}

func subject(ref descruntime.Reference) string {
	return "reference " + ref.Name + " (" + ref.Component + ":" + ref.Version + ")"
}
