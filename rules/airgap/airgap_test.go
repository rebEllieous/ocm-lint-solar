package airgap_test

import (
	"context"
	"testing"

	"github.com/rebEllieous/ocm-lint-solar/internal/fixture"
	"github.com/rebEllieous/ocm-lint-solar/lint"
	_ "github.com/rebEllieous/ocm-lint-solar/rules/airgap"
)

func run(t *testing.T, id string, tg *lint.Target) []lint.Finding {
	t.Helper()
	for _, r := range lint.Default.All() {
		if r.ID() == id {
			f, err := r.Check(context.Background(), tg)
			if err != nil {
				t.Fatalf("%s: %v", id, err)
			}
			return f
		}
	}
	t.Fatalf("rule %s not registered", id)
	return nil
}

var registryLoc = lint.Location{Spec: "reg.local:5000/catalog", Host: "reg.local:5000", Namespace: "catalog"}

func TestNoExternalAccess(t *testing.T) {
	oci := fixture.OCIResource
	tests := []struct {
		name  string
		res   string
		loc   lint.Location
		allow []string
		count int
	}{
		{"same host", oci("i", "ociImage", "reg.local:5000/other/app:1"), registryLoc, nil, 0},
		{"reanchored path", oci("i", "ociImage", "localhost:4443/catalog/app:1"), registryLoc, nil, 0},
		{"external", oci("i", "ociImage", "ghcr.io/acme/app:1"), registryLoc, nil, 1},
		{"allowed", oci("i", "ociImage", "ghcr.io/acme/app:1"), registryLoc, []string{"ghcr.io"}, 0},
		{"docker hub short name", oci("i", "ociImage", "nginx:1.25"), registryLoc, nil, 1},
		{"digest only", oci("i", "ociImage", "ghcr.io/acme/app@sha256:"+zeros), registryLoc, nil, 1},
		{"ctf has no host", oci("i", "ociImage", "reg.local:5000/catalog/app:1"), lint.Location{Spec: "ctf::x"}, nil, 1},
		{"unparsable reference", oci("i", "ociImage", "UPPER CASE::bad"), registryLoc, nil, 1},
		{"global access checked", `{"name":"g","version":"1.0.0","type":"ociImage","relation":"local","access":{"type":"localBlob","localReference":"sha256:00","mediaType":"application/vnd.oci.image.manifest.v1+tar","referenceName":"x","globalAccess":{"type":"ociArtifact","imageReference":"ghcr.io/acme/app:1"}}}`, registryLoc, nil, 1},
		{"local blob alone is fine", fixture.LocalBlobResource("v", "yaml", "application/x-yaml", "", ""), registryLoc, nil, 0},
		{"helm repository access", `{"name":"h","version":"1.0.0","type":"helmChart","relation":"external","access":{"type":"helm/v1","helmRepository":"https://charts.example.com","helmChart":"app:1.0.0"}}`, registryLoc, nil, 1},
		{"wget access", `{"name":"w","version":"1.0.0","type":"blob","relation":"external","access":{"type":"wget/v1","url":"https://example.com/x.tgz"}}`, registryLoc, nil, 1},
		{"ociBlob external", `{"name":"b","version":"1.0.0","type":"blob","relation":"external","access":{"type":"ociBlob","imageReference":"ghcr.io/acme/app:1","digest":"sha256:` + "0000000000000000000000000000000000000000000000000000000000000000" + `","mediaType":"application/octet-stream","size":1}}`, registryLoc, nil, 1},
		{"ociBlob internal", `{"name":"b","version":"1.0.0","type":"blob","relation":"external","access":{"type":"ociBlob","imageReference":"reg.local:5000/catalog/app:1","digest":"sha256:` + "0000000000000000000000000000000000000000000000000000000000000000" + `","mediaType":"application/octet-stream","size":1}}`, registryLoc, nil, 0},
		{"relative access is left to no-relative-access", `{"name":"r","version":"1.0.0","type":"ociImage","relation":"local","access":{"type":"relativeOciReference/v1","reference":"a/b:1"}}`, registryLoc, nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := fixture.Descriptor(t, fixture.Component("acme.org/app", "1.0.0", tt.res, ""))
			tg := lint.NewTarget(d, fixture.Repo(), tt.loc, lint.Options{AllowHosts: tt.allow})
			if got := run(t, "air-gap/no-external-access", tg); len(got) != tt.count {
				t.Fatalf("%d findings, want %d: %+v", len(got), tt.count, got)
			}
		})
	}
}

var zeros = "0000000000000000000000000000000000000000000000000000000000000000"

func TestNoRelativeAccessAndReferenceName(t *testing.T) {
	rel := `{"name":"r","version":"1.0.0","type":"ociImage","relation":"local","access":{"type":"relativeOciReference/v1","reference":"a/b:1"}}`
	ociBlobNoName := fixture.LocalBlobResource("c", "helmChart", "application/vnd.oci.image.manifest.v1+tar+gzip", "", "")
	ociBlobNamed := fixture.LocalBlobResource("c", "helmChart", "application/vnd.oci.image.manifest.v1+tar+gzip", "acme/chart", "")
	yamlBlob := fixture.LocalBlobResource("v", "yaml", "application/x-yaml", "", "")

	tests := []struct {
		rule, res string
		count     int
	}{
		{"air-gap/no-relative-access", rel, 1},
		{"air-gap/no-relative-access", yamlBlob, 0},
		{"air-gap/reference-name-set", ociBlobNoName, 1},
		{"air-gap/reference-name-set", ociBlobNamed, 0},
		{"air-gap/reference-name-set", yamlBlob, 0}, // not OCI content
	}
	for _, tt := range tests {
		t.Run(tt.rule, func(t *testing.T) {
			d := fixture.Descriptor(t, fixture.Component("acme.org/app", "1.0.0", tt.res, ""))
			tg := lint.NewTarget(d, fixture.Repo(), registryLoc, lint.Options{})
			if got := run(t, tt.rule, tg); len(got) != tt.count {
				t.Fatalf("%d findings, want %d: %+v", len(got), tt.count, got)
			}
		})
	}
}

func TestReferencesClosed(t *testing.T) {
	c, r := fixture.Component, fixture.Ref
	// app -> a -> b -> missing, b -> app (cycle); app -> gone (direct, not reported here)
	app := fixture.Descriptor(t, c("acme.org/app", "1", "", r("a", "acme.org/a", "1")+","+r("g", "acme.org/gone", "1")))
	a := fixture.Descriptor(t, c("acme.org/a", "1", "", r("b", "acme.org/b", "1")))
	b := fixture.Descriptor(t, c("acme.org/b", "1", "", r("m", "acme.org/missing", "1")+","+r("app", "acme.org/app", "1")))
	tg := lint.NewTarget(app, fixture.Repo(app, a, b), registryLoc, lint.Options{})

	got := run(t, "air-gap/references-closed", tg)
	if len(got) != 1 || got[0].Subject != "reference acme.org/app:1 -> acme.org/a:1 -> acme.org/b:1 -> acme.org/missing:1" {
		t.Fatalf("findings = %+v", got)
	}
}
