package lint

import (
	"context"
	"fmt"
	"os"
	"strings"

	"ocm.software/open-component-model/bindings/go/oci"
	ocirepo "ocm.software/open-component-model/bindings/go/oci/repository"
	ctfspec "ocm.software/open-component-model/bindings/go/oci/spec/repository/v1/ctf"
	ocispec "ocm.software/open-component-model/bindings/go/oci/spec/repository/v1/oci"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
	"oras.land/oras-go/v2/registry/remote/retry"
)

// Ref is a parsed "<repo>//<component>:<version>".
type Ref struct {
	Repo, Component, Version string
}

func ParseRef(s string) (Ref, error) {
	// Split on the last "//": a scheme ("http://") also contains one.
	i := strings.LastIndex(s, "//")
	if i <= 0 {
		return Ref{}, fmt.Errorf("ref %q: want <repo>//<component>:<version>", s)
	}
	repo, cv := s[:i], s[i+2:]
	j := strings.LastIndex(cv, ":")
	if j <= 0 || j == len(cv)-1 {
		return Ref{}, fmt.Errorf("ref %q: missing component or version", s)
	}
	return Ref{Repo: repo, Component: cv[:j], Version: cv[j+1:]}, nil
}

// Repository is an opened CTF or OCI repository.
type Repository struct {
	Location Location
	repo     *oci.Repository
	tempDir  string
}

// Open opens "ctf::<path>" or an OCI repository "[http(s)://]<host>[/<namespace>]".
// OCI credentials come from the Docker config.
func Open(ctx context.Context, spec string) (*Repository, error) {
	tempDir, err := os.MkdirTemp("", "ocm-lint-")
	if err != nil {
		return nil, err
	}
	r := &Repository{Location: Location{Spec: spec}, tempDir: tempDir}

	if path, ok := strings.CutPrefix(spec, "ctf::"); ok {
		r.repo, err = ocirepo.NewFromCTFRepoV1(ctx, &ctfspec.Repository{
			FilePath: path, AccessMode: ctfspec.AccessModeReadOnly,
		}, oci.WithTempDir(tempDir))
	} else {
		hostPath := spec
		if _, rest, found := strings.Cut(spec, "://"); found {
			hostPath = rest
		}
		r.Location.Host, r.Location.Namespace, _ = strings.Cut(strings.TrimSuffix(hostPath, "/"), "/")

		var client *auth.Client
		client, err = dockerClient()
		if err == nil {
			r.repo, err = ocirepo.NewFromOCIRepoV1(ctx, &ocispec.Repository{BaseUrl: spec}, client, oci.WithTempDir(tempDir))
		}
	}
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, fmt.Errorf("open %s: %w", spec, err)
	}
	return r, nil
}

func dockerClient() (*auth.Client, error) {
	store, err := credentials.NewStoreFromDocker(credentials.StoreOptions{})
	if err != nil {
		return nil, fmt.Errorf("load docker config: %w", err)
	}
	return &auth.Client{Client: retry.DefaultClient, Cache: auth.NewCache(), Credential: credentials.Credential(store)}, nil
}

// Target loads a component version as a lint target.
func (r *Repository) Target(ctx context.Context, component, version string, opts Options) (*Target, error) {
	d, err := r.repo.GetComponentVersion(ctx, component, version)
	if err != nil {
		return nil, fmt.Errorf("load %s:%s: %w", component, version, err)
	}
	return NewTarget(d, r.repo, r.Location, opts), nil
}

func (r *Repository) Close() error { return os.RemoveAll(r.tempDir) }
