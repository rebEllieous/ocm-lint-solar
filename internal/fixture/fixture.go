// Package fixture builds OCM test components without the ocm binary.
package fixture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	descruntime "ocm.software/open-component-model/bindings/go/descriptor/runtime"
	descv2 "ocm.software/open-component-model/bindings/go/descriptor/v2"
	"ocm.software/open-component-model/bindings/go/oci"
	ocirepo "ocm.software/open-component-model/bindings/go/oci/repository"
	ctfspec "ocm.software/open-component-model/bindings/go/oci/spec/repository/v1/ctf"
	ocispec "ocm.software/open-component-model/bindings/go/oci/spec/repository/v1/oci"
	"ocm.software/open-component-model/bindings/go/repository"
	"oras.land/oras-go/v2/registry/remote/auth"
)

// Component returns a schema-valid v2 descriptor. resources and refs are JSON
// array bodies (without brackets), e.g. `{"name":"a",...},{"name":"b",...}`.
func Component(name, version, resources, refs string) string {
	return fmt.Sprintf(`{
  "meta": {"schemaVersion": "v2"},
  "component": {
    "name": %q, "version": %q, "provider": "acme",
    "repositoryContexts": [], "sources": [],
    "resources": [%s],
    "componentReferences": [%s]
  }
}`, name, version, resources, refs)
}

// OCIResource is a resource with OCIImage access.
func OCIResource(name, typ, imageRef string) string {
	return fmt.Sprintf(`{"name":%q,"version":"1.0.0","type":%q,"relation":"external",
"access":{"type":"OCIImage/v1","imageReference":%q}}`, name, typ, imageRef)
}

// LocalBlobResource is a resource with localBlob access. labels is a JSON array body.
func LocalBlobResource(name, typ, mediaType, referenceName, labels string) string {
	return fmt.Sprintf(`{"name":%q,"version":"1.0.0","type":%q,"relation":"local","labels":[%s],
"access":{"type":"localBlob/v1","localReference":"sha256:%s","mediaType":%q,"referenceName":%q}}`,
		name, typ, labels, strings.Repeat("0", 64), mediaType, referenceName)
}

// Ref is a component reference.
func Ref(name, component, version string) string {
	return fmt.Sprintf(`{"name":%q,"componentName":%q,"version":%q}`, name, component, version)
}

// Descriptor decodes a v2 JSON descriptor into the runtime form rules see.
func Descriptor(t testing.TB, js string) *descruntime.Descriptor {
	t.Helper()
	var v2 descv2.Descriptor
	if err := json.Unmarshal([]byte(js), &v2); err != nil {
		t.Fatalf("decode descriptor: %v", err)
	}
	d, err := descruntime.ConvertFromV2(&v2)
	if err != nil {
		t.Fatalf("convert descriptor: %v", err)
	}
	return d
}

// CTF writes the descriptors into a new CTF directory and returns "ctf::<dir>".
func CTF(t testing.TB, ds ...*descruntime.Descriptor) string {
	t.Helper()
	dir := t.TempDir()
	repo, err := ocirepo.NewFromCTFRepoV1(context.Background(), &ctfspec.Repository{
		FilePath:   dir,
		AccessMode: ctfspec.AccessModeReadWrite + "|" + ctfspec.AccessModeCreate,
	}, oci.WithTempDir(t.TempDir()))
	if err != nil {
		t.Fatalf("open CTF: %v", err)
	}
	add(t, repo, ds)
	return "ctf::" + dir
}

// Registry starts an in-process OCI registry holding the descriptors under
// namespace "test" and returns "http://<host>/test".
func Registry(t testing.TB, ds ...*descruntime.Descriptor) string {
	t.Helper()
	srv := httptest.NewServer(registry.New())
	t.Cleanup(srv.Close)
	url := "http://" + strings.TrimPrefix(srv.URL, "http://") + "/test"
	repo, err := ocirepo.NewFromOCIRepoV1(context.Background(), &ocispec.Repository{BaseUrl: url},
		&auth.Client{}, oci.WithTempDir(t.TempDir()))
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}
	add(t, repo, ds)
	return url
}

func add(t testing.TB, repo *oci.Repository, ds []*descruntime.Descriptor) {
	t.Helper()
	for _, d := range ds {
		if err := repo.AddComponentVersion(context.Background(), d); err != nil {
			t.Fatalf("add %s:%s: %v", d.Component.Name, d.Component.Version, err)
		}
	}
}

// FakeRepo is an in-memory Getter keyed by "name:version".
type FakeRepo map[string]*descruntime.Descriptor

func (f FakeRepo) GetComponentVersion(_ context.Context, component, version string) (*descruntime.Descriptor, error) {
	if d, ok := f[component+":"+version]; ok {
		return d, nil
	}
	return nil, errors.Join(repository.ErrNotFound, fmt.Errorf("%s:%s", component, version))
}

// Repo indexes descriptors into a FakeRepo.
func Repo(ds ...*descruntime.Descriptor) FakeRepo {
	f := FakeRepo{}
	for _, d := range ds {
		f[d.Component.Name+":"+d.Component.Version] = d
	}
	return f
}
