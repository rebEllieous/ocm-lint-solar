package lint_test

import (
	"fmt"
	"testing"

	"github.com/rebEllieous/ocm-lint-solar/internal/fixture"
	"github.com/rebEllieous/ocm-lint-solar/lint"
)

func sprintf(format string, a ...any) string { return fmt.Sprintf(format, a...) }

func TestAccessOf(t *testing.T) {
	res := `{"name":"r","version":"1.0.0","type":"blob","relation":"local","access":%s}`
	tests := []struct {
		name                          string
		access                        string
		oci, local, relative, content bool
		global                        bool
	}{
		{name: "OCIImage versioned", access: `{"type":"OCIImage/v1","imageReference":"ghcr.io/a/b:1"}`, oci: true},
		{name: "ociArtifact legacy", access: `{"type":"ociArtifact","imageReference":"ghcr.io/a/b:1"}`, oci: true},
		{name: "relative", access: `{"type":"relativeOciReference/v1","reference":"a/b:1"}`, relative: true},
		{name: "localBlob yaml", access: `{"type":"localBlob/v1","localReference":"sha256:00","mediaType":"application/x-yaml"}`, local: true},
		{name: "LocalBlob OCI content", access: `{"type":"LocalBlob","localReference":"sha256:00","mediaType":"application/vnd.oci.image.manifest.v1+tar+gzip"}`, local: true, content: true},
		{name: "localBlob with globalAccess", access: `{"type":"localBlob","localReference":"sha256:00","mediaType":"application/vnd.oci.image.index.v1+json","globalAccess":{"type":"ociArtifact","imageReference":"ghcr.io/a/b:1"}}`, local: true, content: true, global: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := fixture.Descriptor(t, fixture.Component("acme.org/app", "1.0.0", sprintf(res, tt.access), ""))
			a, err := lint.AccessOf(d.Component.Resources[0].Access)
			if err != nil {
				t.Fatal(err)
			}
			if a.IsOCI() != tt.oci || a.IsLocalBlob() != tt.local || a.IsRelative() != tt.relative || a.HoldsOCIContent() != tt.content {
				t.Fatalf("predicates wrong for %+v", a)
			}
			g, ok := a.Global()
			if ok != tt.global || (ok && (!g.IsOCI() || g.ImageReference != "ghcr.io/a/b:1")) {
				t.Fatalf("Global() = %+v, %v", g, ok)
			}
		})
	}
}

func TestLabelString(t *testing.T) {
	labels := `{"name":"s","value":"chart"},{"name":"n","value":42}`
	d := fixture.Descriptor(t, fixture.Component("acme.org/app", "1.0.0",
		fixture.LocalBlobResource("r", "yaml", "application/x-yaml", "", labels), ""))
	ls := d.Component.Resources[0].Labels
	if v, ok := lint.LabelString(ls, "s"); !ok || v != "chart" {
		t.Fatalf("s = %q, %v", v, ok)
	}
	if _, ok := lint.LabelString(ls, "n"); ok {
		t.Fatal("non-string label value reported as string")
	}
	if _, ok := lint.LabelString(ls, "missing"); ok {
		t.Fatal("missing label reported")
	}
}
