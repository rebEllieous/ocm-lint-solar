package lint

import (
	"context"
	"slices"
	"testing"
)

func fake(id string) Rule {
	return Func{RuleID: id, Sev: Error, Desc: id, Fn: func(context.Context, *Target) ([]Finding, error) { return nil, nil }}
}

func ids(rs []Rule) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.ID()
	}
	return out
}

func TestSelect(t *testing.T) {
	g := &Registry{}
	for _, id := range []string{"core/a", "core/b", "air-gap/x", "solar/y", "solar/z"} {
		g.Register(fake(id))
	}

	tests := []struct {
		name          string
		include, skip []string
		want          []string
		wantErr       bool
	}{
		{name: "default is core", want: []string{"core/a", "core/b"}},
		{name: "add pack", include: []string{"solar"}, want: []string{"core/a", "core/b", "solar/y", "solar/z"}},
		{name: "single rule", include: []string{"air-gap/x"}, want: []string{"air-gap/x", "core/a", "core/b"}},
		{name: "skip", include: []string{"solar"}, skip: []string{"solar/y", "core/a"}, want: []string{"core/b", "solar/z"}},
		{name: "skip known but unselected rule is a no-op", skip: []string{"solar/y"}, want: []string{"core/a", "core/b"}},
		{name: "unknown pack", include: []string{"nope"}, wantErr: true},
		{name: "unknown rule", include: []string{"solar/nope"}, wantErr: true},
		{name: "unknown skip", skip: []string{"core/nope"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := g.Select(tt.include, tt.skip)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && !slices.Equal(ids(got), tt.want) {
				t.Fatalf("got %v, want %v", ids(got), tt.want)
			}
		})
	}
}

func TestRegisterRejectsBadAndDuplicateIDs(t *testing.T) {
	for _, id := range []string{"nopack", "/x", "core/", "dup/a"} {
		t.Run(id, func(t *testing.T) {
			g := &Registry{}
			g.Register(fake("dup/a"))
			defer func() {
				if recover() == nil {
					t.Fatalf("Register(%q) did not panic", id)
				}
			}()
			g.Register(fake(id))
		})
	}
}
