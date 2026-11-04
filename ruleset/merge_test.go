package ruleset_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/rebEllieous/ocm-lint-solar/lint"
	"github.com/rebEllieous/ocm-lint-solar/ruleset"
)

func ids(rs []lint.Rule) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.ID()
	}
	return out
}

func rule(id, desc string) string { return spec("id", id, "description", desc) }

func TestLoadMerge(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		want     []string
		wantDesc string // description of the first rule, when set
		warn     string // substring of the only warning, when set
	}{
		{name: "later file replaces", files: []string{rule("solar/a", "old"), rule("solar/a", "new")}, want: []string{"solar/a"}, wantDesc: "new"},
		{name: "last of three wins", files: []string{rule("solar/a", "1"), rule("solar/a", "2"), rule("solar/a", "3")}, want: []string{"solar/a"}, wantDesc: "3"},
		{name: "typo keeps both and warns", files: []string{rule("solar/a", "d"), rule("solar/a-typo", "d")}, want: []string{"solar/a", "solar/a-typo"}, warn: "solar/a-typo"},
		{name: "new pack is quiet", files: []string{rule("solar/a", "d"), rule("acme/b", "d")}, want: []string{"acme/b", "solar/a"}},
		{name: "first use of a pack is quiet", files: []string{rule("acme/b", "d"), rule("solar/a", "d"), rule("acme/b", "d")}, want: []string{"acme/b", "solar/a"}},
		{name: "sorted", files: []string{rule("z/a", "d"), rule("a/a", "d")}, want: []string{"a/a", "z/a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var paths []string
			for _, f := range tt.files {
				paths = append(paths, file(t, f))
			}
			got, warnings, err := ruleset.Load(context.Background(), paths)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(ids(got), tt.want) {
				t.Fatalf("got %v, want %v", ids(got), tt.want)
			}
			if tt.wantDesc != "" && got[0].Description() != tt.wantDesc {
				t.Fatalf("description %q, want %q", got[0].Description(), tt.wantDesc)
			}
			if tt.warn == "" {
				if len(warnings) != 0 {
					t.Fatalf("warnings: %v", warnings)
				}
				return
			}
			if len(warnings) != 1 || !strings.Contains(warnings[0], tt.warn) || !strings.Contains(warnings[0], paths[1]) {
				t.Fatalf("warnings: %v", warnings)
			}
		})
	}
}

func TestLoadSameFileTwice(t *testing.T) {
	p := file(t, rule("solar/a", "d"))
	got, warnings, err := ruleset.Load(context.Background(), []string{p, p})
	if err != nil || len(warnings) != 0 || !slices.Equal(ids(got), []string{"solar/a"}) {
		t.Fatalf("rules %v, warnings %v, err %v", ids(got), warnings, err)
	}
}

func TestLoadSeveralNewRulesInOnePackFirstFile(t *testing.T) {
	two := rule("solar/a", "d") + "- id: solar/b\n  severity: warn\n  description: d\n  check: \"true\"\n  message: '\"m\"'\n"
	_, warnings, err := ruleset.Load(context.Background(), []string{file(t, two)})
	if err != nil || len(warnings) != 0 {
		t.Fatalf("warnings %v, err %v", warnings, err)
	}
}

func TestSkip(t *testing.T) {
	rules, _, err := ruleset.Load(context.Background(), []string{file(t, rule("solar/a", "1")), file(t, rule("solar/a", "2")), file(t, rule("acme/b", "d"))})
	if err != nil {
		t.Fatal(err)
	}
	got, err := ruleset.Skip(rules, []string{"solar/a"})
	if err != nil || !slices.Equal(ids(got), []string{"acme/b"}) {
		t.Fatalf("rules %v, err %v", ids(got), err)
	}
	if _, err := ruleset.Skip(rules, []string{"solar/nope"}); err == nil {
		t.Fatal("unknown id: no error")
	}
}
