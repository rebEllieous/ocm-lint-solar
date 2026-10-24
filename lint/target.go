package lint

import (
	"context"

	descruntime "ocm.software/open-component-model/bindings/go/descriptor/runtime"
)

// Getter is the read side of an OCM repository.
type Getter interface {
	GetComponentVersion(ctx context.Context, component, version string) (*descruntime.Descriptor, error)
}

// Location says where a component was read from.
type Location struct {
	Spec      string // repo part of the ref, e.g. "ctf::./out" or "ghcr.io/org"
	Host      string // empty for CTF
	Namespace string // empty for CTF or a registry root
}

// Options are rule inputs from the command line.
type Options struct {
	AllowHosts []string
}

// Target is one component version under lint.
type Target struct {
	Descriptor *descruntime.Descriptor
	Location   Location
	Options    Options
	refs       *refCache
}

type refCache struct {
	repo Getter
	m    map[string]refEntry
}

type refEntry struct {
	d   *descruntime.Descriptor
	err error
}

func NewTarget(d *descruntime.Descriptor, repo Getter, loc Location, opts Options) *Target {
	return &Target{Descriptor: d, Location: loc, Options: opts, refs: &refCache{repo: repo, m: map[string]refEntry{}}}
}

// Resolve fetches a referenced component from the target's repository. Results,
// including errors, are cached and shared with every Child.
func (t *Target) Resolve(ctx context.Context, component, version string) (*descruntime.Descriptor, error) {
	key := component + ":" + version
	if e, ok := t.refs.m[key]; ok {
		return e.d, e.err
	}
	d, err := t.refs.repo.GetComponentVersion(ctx, component, version)
	t.refs.m[key] = refEntry{d, err}
	return d, err
}

// Child is a target for a referenced component in the same repository.
func (t *Target) Child(d *descruntime.Descriptor) *Target {
	return &Target{Descriptor: d, Location: t.Location, Options: t.Options, refs: t.refs}
}

func (t *Target) Name() string {
	return t.Descriptor.Component.Name + ":" + t.Descriptor.Component.Version
}

// Key identifies the target across repositories within one invocation.
func (t *Target) Key() string { return t.Location.Spec + "//" + t.Name() }
