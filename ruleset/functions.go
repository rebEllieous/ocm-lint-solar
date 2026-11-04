package ruleset

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/common/functions"
	"cel.dev/cel-go/common/types"
	"cel.dev/cel-go/common/types/ref"
	"github.com/google/go-containerregistry/pkg/name"
	descruntime "ocm.software/open-component-model/bindings/go/descriptor/runtime"
	descv2 "ocm.software/open-component-model/bindings/go/descriptor/v2"
	"ocm.software/open-component-model/bindings/go/repository"

	"github.com/rebEllieous/ocm-lint-solar/lint"
)

// functionDecls declares what CEL can't do itself: run the OCM validators, reach
// the repository, and parse image references.
func functionDecls() []cel.EnvOption {
	problems := cel.ListType(cel.ObjectType("ruleset.Problem"))
	var out []cel.EnvOption
	for _, fn := range []string{"schemaErrors", "identityErrors", "unresolvedReferences", "unresolvedTransitiveReferences"} {
		out = append(out, cel.Function(fn, cel.Overload(fn, nil, problems)))
	}
	return append(out, cel.Function("externalReason", cel.Overload("externalReason", []*cel.Type{cel.StringType}, cel.StringType)))
}

// bindings implements the functions for one target. Programs are created per
// Check so the implementations can capture the target and context.
func bindings(ctx context.Context, t *lint.Target, adapter types.Adapter) cel.ProgramOption {
	list := func(name string, fn func() ([]Problem, error)) *functions.Overload {
		return &functions.Overload{Operator: name, Function: func(...ref.Val) ref.Val {
			ps, err := fn()
			if err != nil {
				return types.WrapErr(err)
			}
			return adapter.NativeToValue(ps)
		}}
	}
	//nolint:staticcheck // its replacement binds once per Env, but these capture the target and ctx of each Check
	return cel.Functions(
		list("schemaErrors", func() ([]Problem, error) { return schemaErrors(t), nil }),
		list("identityErrors", func() ([]Problem, error) { return identityErrors(t), nil }),
		list("unresolvedReferences", func() ([]Problem, error) { return unresolvedReferences(ctx, t) }),
		list("unresolvedTransitiveReferences", func() ([]Problem, error) { return unresolvedTransitiveReferences(ctx, t) }),
		&functions.Overload{Operator: "externalReason", Unary: func(v ref.Val) ref.Val {
			return types.String(externalReason(v.Value().(string), t))
		}},
	)
}

func schemaErrors(t *lint.Target) []Problem {
	v2, err := descruntime.ConvertToV2(descv2.Scheme, t.Descriptor)
	if err != nil {
		return []Problem{{"descriptor", fmt.Sprintf("cannot convert to v2: %v", err)}}
	}
	if err := descv2.Validate(v2); err != nil {
		return []Problem{{"descriptor", err.Error()}}
	}
	return []Problem{}
}

func identityErrors(t *lint.Target) []Problem {
	if err := descruntime.Validate(t.Descriptor); err != nil {
		return []Problem{{"descriptor", err.Error()}}
	}
	return []Problem{}
}

func unresolvedReferences(ctx context.Context, t *lint.Target) ([]Problem, error) {
	ps := []Problem{}
	for _, ref := range t.Descriptor.Component.References {
		_, err := t.Resolve(ctx, ref.Component, ref.Version)
		switch {
		case errors.Is(err, repository.ErrNotFound):
			ps = append(ps, Problem{
				"reference " + ref.Name + " (" + ref.Component + ":" + ref.Version + ")",
				fmt.Sprintf("%s:%s not found in %s", ref.Component, ref.Version, t.Location.Spec),
			})
		case err != nil:
			return nil, fmt.Errorf("resolve %s:%s: %w", ref.Component, ref.Version, err)
		}
	}
	return ps, nil
}

// unresolvedTransitiveReferences walks the reference graph and reports unresolvable
// references at depth 2 or deeper; direct ones are unresolvedReferences.
func unresolvedTransitiveReferences(ctx context.Context, t *lint.Target) ([]Problem, error) {
	type node struct {
		d    *lint.Target
		path []string
	}
	ps := []Problem{}
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
					ps = append(ps, Problem{"reference " + strings.Join(path, " -> "), "not found in " + t.Location.Spec})
				}
			case err != nil:
				return nil, fmt.Errorf("resolve %s: %w", key, err)
			case !seen[key]:
				seen[key] = true
				queue = append(queue, node{t.Child(d), path})
			}
		}
	}
	return ps, nil
}

// externalReason returns why ref is outside the repository, or "" when it is inside.
func externalReason(ref string, t *lint.Target) string {
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
