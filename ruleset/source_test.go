package ruleset_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rebEllieous/ocm-lint-solar/lint"
	"github.com/rebEllieous/ocm-lint-solar/ruleset"
)

// Adding a rule file is how people build in their own rules, so more is fine.
func TestAliases(t *testing.T) {
	got := ruleset.Aliases()
	if !slices.IsSorted(got) {
		t.Fatalf("not sorted: %v", got)
	}
	for _, want := range []string{"@air-gap", "@core", "@solar"} {
		if !slices.Contains(got, want) {
			t.Fatalf("%s not in %v", want, got)
		}
	}
}

func metadata(rules []lint.Rule) []string {
	var out []string
	for _, r := range rules {
		out = append(out, r.ID()+"|"+r.Severity().String()+"|"+r.Description())
	}
	return out
}

func TestAliasMatchesFile(t *testing.T) {
	for _, name := range []string{"core", "air-gap", "solar"} {
		t.Run(name, func(t *testing.T) {
			alias, warnings, err := ruleset.Load(context.Background(), []string{"@" + name})
			if err != nil || len(warnings) != 0 {
				t.Fatalf("warnings %v, err %v", warnings, err)
			}
			path, _, err := ruleset.Load(context.Background(), []string{filepath.Join("..", "rules", name+".yaml")})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(metadata(alias), metadata(path)) {
				t.Fatalf("alias %v, file %v", metadata(alias), metadata(path))
			}
		})
	}
}

func TestAliasNeedsNoFiles(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, _, err := ruleset.Load(context.Background(), []string{"@core"}); err != nil {
		t.Fatal(err)
	}
}

func TestUnknownAlias(t *testing.T) {
	for _, source := range []string{"@", "@nope", "@../core", "@core.yaml", "@CORE", "@./core"} {
		t.Run(source, func(t *testing.T) {
			_, _, err := ruleset.Load(context.Background(), []string{source})
			if err == nil {
				t.Fatal("no error")
			}
			if !strings.Contains(err.Error(), source) || !strings.Contains(err.Error(), "@solar") {
				t.Fatalf("error should name the source and the aliases: %v", err)
			}
		})
	}
}

func TestPathStartingWithAt(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("@x.yaml", []byte(spec()), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ruleset.Load(context.Background(), []string{"./@x.yaml"}); err != nil {
		t.Fatal(err)
	}
}

func TestAliasMergesWithFiles(t *testing.T) {
	override := file(t, rule("core/schema-valid", "overridden"))
	description := func(sources ...string) string {
		t.Helper()
		rules, _, err := ruleset.Load(context.Background(), sources)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rules {
			if r.ID() == "core/schema-valid" {
				return r.Description()
			}
		}
		t.Fatal("rule not loaded")
		return ""
	}
	if got := description("@core", override); got != "overridden" {
		t.Fatalf("later file should win: %q", got)
	}
	if got := description(override, "@core"); got == "overridden" {
		t.Fatal("later alias should win")
	}
}

// serve returns a server answering every request with body and status, and its URL.
func serve(t *testing.T, status int, body string) string {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(s.Close)
	return s.URL + "/rules.yaml"
}

func TestLoadURL(t *testing.T) {
	u := serve(t, http.StatusOK, spec())
	rules, warnings, err := ruleset.Load(context.Background(), []string{u})
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].ID() != "t/r" {
		t.Fatalf("rules: %v", metadata(rules))
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], u) {
		t.Fatalf("warnings: %v", warnings)
	}
}

func TestAliasAndPathGiveNoNotice(t *testing.T) {
	_, warnings, err := ruleset.Load(context.Background(), []string{"@core", file(t, spec())})
	if err != nil || len(warnings) != 0 {
		t.Fatalf("warnings %v, err %v", warnings, err)
	}
}

func TestURLOverridesAlias(t *testing.T) {
	u := serve(t, http.StatusOK, rule("core/schema-valid", "overridden"))
	description := func(sources ...string) string {
		t.Helper()
		rules, _, err := ruleset.Load(context.Background(), sources)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rules {
			if r.ID() == "core/schema-valid" {
				return r.Description()
			}
		}
		t.Fatal("rule not loaded")
		return ""
	}
	if got := description("@core", u); got != "overridden" {
		t.Fatalf("later URL should win: %q", got)
	}
	if got := description(u, "@core"); got == "overridden" {
		t.Fatal("later alias should win")
	}
}

func TestURLFailures(t *testing.T) {
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	tests := []struct {
		name, url, want string
	}{
		{"404", serve(t, http.StatusNotFound, spec()), "404"},
		{"500", serve(t, http.StatusInternalServerError, spec()), "500"},
		{"github page", serve(t, http.StatusOK, "<!DOCTYPE html><html></html>"), ""},
		{"no rules", serve(t, http.StatusOK, "rules: []\n"), ""},
		{"invalid yaml", serve(t, http.StatusOK, "rules: [\n"), ""},
		{"unreachable", closed.URL + "/rules.yaml", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules, _, err := ruleset.Load(context.Background(), []string{tt.url})
			if err == nil {
				t.Fatal("no error")
			}
			if rules != nil || !strings.Contains(err.Error(), tt.url) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("rules %v, err %v", rules, err)
			}
		})
	}
}

func TestURLSizeLimit(t *testing.T) {
	const limit = 1 << 20
	sized := func(n int) string {
		t.Helper()
		// Flushing before the end means no Content-Length, so only the bytes read count.
		body := spec() + "#" + strings.Repeat("x", n-len(spec())-1)
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.(http.Flusher).Flush()
			_, _ = io.WriteString(w, body)
		}))
		t.Cleanup(s.Close)
		return s.URL
	}
	if _, _, err := ruleset.Load(context.Background(), []string{sized(limit)}); err != nil {
		t.Fatalf("at the limit: %v", err)
	}
	u := sized(limit + 1)
	if _, _, err := ruleset.Load(context.Background(), []string{u}); err == nil || !strings.Contains(err.Error(), u) {
		t.Fatalf("over the limit: %v", err)
	}
}

func TestURLTimeout(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	t.Cleanup(s.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, _, err := ruleset.Load(ctx, []string{s.URL}); err == nil || !strings.Contains(err.Error(), s.URL) {
		t.Fatalf("err = %v", err)
	}
}

func TestURLSecretsNotPrinted(t *testing.T) {
	var query string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		if r.URL.Path == "/missing" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, spec())
	}))
	t.Cleanup(s.Close)
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	withUser := strings.Replace(s.URL, "http://", "http://u:s3cret@", 1)
	closedWithUser := strings.Replace(closed.URL, "http://", "http://u:s3cret@", 1)

	for _, u := range []string{s.URL + "/?token=s3cret", withUser + "/", s.URL + "/missing?token=s3cret", withUser + "/missing", closed.URL + "/?token=s3cret", closedWithUser + "/"} {
		t.Run(u, func(t *testing.T) {
			_, warnings, err := ruleset.Load(context.Background(), []string{u})
			if strings.Contains(fmt.Sprint(warnings, err), "s3cret") {
				t.Fatalf("secret printed: warnings %v, err %v", warnings, err)
			}
		})
	}
	if _, _, err := ruleset.Load(context.Background(), []string{s.URL + "/?token=s3cret"}); err != nil {
		t.Fatal(err)
	}
	if query != "token=s3cret" {
		t.Fatalf("query not sent: %q", query)
	}
}

func TestInvalidURL(t *testing.T) {
	_, _, err := ruleset.Load(context.Background(), []string{"http://%zz"})
	if err == nil || strings.Contains(err.Error(), "%zz") {
		t.Fatalf("err = %v", err)
	}
}

func TestOtherSchemesAreRejectedWithoutPrintingSecrets(t *testing.T) {
	for _, source := range []string{"HTTP://host/x?token=s3cret", "ftp://u:s3cret@host/x"} {
		t.Run(source, func(t *testing.T) {
			_, _, err := ruleset.Load(context.Background(), []string{source})
			if err == nil || strings.Contains(err.Error(), "s3cret") || !strings.Contains(err.Error(), "https://") {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestRedirectDoesNotSendTheURLToTheTarget(t *testing.T) {
	var referer string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		referer = r.Referer()
		_, _ = io.WriteString(w, spec())
	}))
	t.Cleanup(target.Close)
	s := httptest.NewServer(http.RedirectHandler(target.URL, http.StatusFound))
	t.Cleanup(s.Close)
	if _, _, err := ruleset.Load(context.Background(), []string{s.URL + "/?token=s3cret"}); err != nil {
		t.Fatal(err)
	}
	if referer != "" {
		t.Fatalf("Referer sent: %q", referer)
	}
}
