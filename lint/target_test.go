package lint_test

import (
	"context"
	"errors"
	"testing"

	descruntime "ocm.software/open-component-model/bindings/go/descriptor/runtime"
	"ocm.software/open-component-model/bindings/go/repository"

	"github.com/rebEllieous/ocm-lint-solar/internal/fixture"
	"github.com/rebEllieous/ocm-lint-solar/lint"
)

type countingRepo struct {
	fixture.FakeRepo
	calls int
}

func (c *countingRepo) GetComponentVersion(ctx context.Context, n, v string) (*descruntime.Descriptor, error) {
	c.calls++
	return c.FakeRepo.GetComponentVersion(ctx, n, v)
}

func TestResolveIsCachedAndSharedWithChildren(t *testing.T) {
	dep := fixture.Descriptor(t, fixture.Component("acme.org/dep", "1.0.0", "", ""))
	root := fixture.Descriptor(t, fixture.Component("acme.org/app", "1.0.0", "", fixture.Ref("dep", "acme.org/dep", "1.0.0")))
	repo := &countingRepo{FakeRepo: fixture.Repo(dep)}
	tg := lint.NewTarget(root, repo, lint.Location{Spec: "ctf::x"}, lint.Options{})

	ctx := context.Background()
	d, err := tg.Resolve(ctx, "acme.org/dep", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tg.Child(d).Resolve(ctx, "acme.org/dep", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if repo.calls != 1 {
		t.Fatalf("repo called %d times, want 1", repo.calls)
	}
	if got := tg.Child(d).Key(); got != "ctf::x//acme.org/dep:1.0.0" {
		t.Fatalf("Key = %q", got)
	}

	_, err = tg.Resolve(ctx, "acme.org/missing", "1.0.0")
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
