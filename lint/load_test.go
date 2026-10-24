package lint_test

import (
	"context"
	"testing"

	"github.com/rebEllieous/ocm-lint-solar/internal/fixture"
	"github.com/rebEllieous/ocm-lint-solar/lint"
)

func TestParseRef(t *testing.T) {
	tests := []struct {
		in   string
		want lint.Ref
		err  bool
	}{
		{in: "ctf::./out//acme.org/app:1.0.0", want: lint.Ref{Repo: "ctf::./out", Component: "acme.org/app", Version: "1.0.0"}},
		{in: "ghcr.io/org//acme.org/app:1.0.0", want: lint.Ref{Repo: "ghcr.io/org", Component: "acme.org/app", Version: "1.0.0"}},
		// the scheme's "//" must not be taken as the separator
		{in: "http://localhost:5000/ns//acme.org/app:1.0.0", want: lint.Ref{Repo: "http://localhost:5000/ns", Component: "acme.org/app", Version: "1.0.0"}},
		{in: "ghcr.io/org/acme.org/app:1.0.0", err: true}, // no "//"
		{in: "ghcr.io/org//acme.org/app", err: true},      // no version
		{in: "//acme.org/app:1.0.0", err: true},           // no repo
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := lint.ParseRef(tt.in)
			if (err != nil) != tt.err {
				t.Fatalf("err = %v", err)
			}
			if !tt.err && got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestOpenAndLoad(t *testing.T) {
	d := fixture.Descriptor(t, fixture.Component("acme.org/app", "1.0.0", "", ""))
	for name, spec := range map[string]string{
		"ctf":      fixture.CTF(t, d),
		"registry": fixture.Registry(t, d),
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			repo, err := lint.Open(ctx, spec)
			if err != nil {
				t.Fatal(err)
			}
			defer repo.Close()

			tg, err := repo.Target(ctx, "acme.org/app", "1.0.0", lint.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if tg.Name() != "acme.org/app:1.0.0" {
				t.Fatalf("Name = %q", tg.Name())
			}
			if name == "registry" && (tg.Location.Host == "" || tg.Location.Namespace != "test") {
				t.Fatalf("Location = %+v", tg.Location)
			}
			if _, err := repo.Target(ctx, "acme.org/app", "9.9.9", lint.Options{}); err == nil {
				t.Fatal("loading a missing version did not fail")
			}
		})
	}
}
