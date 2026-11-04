// Command ocm-lint lints OCM component versions.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/rebEllieous/ocm-lint-solar/lint"
	"github.com/rebEllieous/ocm-lint-solar/report"
	"github.com/rebEllieous/ocm-lint-solar/ruleset"
)

const usage = `usage: ocm-lint --rule <file>... [--skip <id>]... [--allow-host <host>]...
                [--recursive] [-o text|json] <repo>//<component>:<version>...
       ocm-lint --rule <file>... [--skip <id>]... --list-rules`

// referencesRule reports references that --recursive and air-gap/references-closed
// leave to it.
const referencesRule = "core/references-resolvable"

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

type list []string

func (l *list) String() string     { return strings.Join(*l, ",") }
func (l *list) Set(v string) error { *l = append(*l, v); return nil }

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ocm-lint", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprintln(stderr, usage) }
	var ruleFiles, skip, allow list
	fs.Var(&ruleFiles, "rule", "rule file; a later file replaces rules with the same ID")
	fs.Var(&skip, "skip", "remove a rule")
	fs.Var(&allow, "allow-host", "registry host treated as inside the repository")
	recursive := fs.Bool("recursive", false, "also lint referenced components")
	output := fs.String("o", "text", "output: text or json")
	listRules := fs.Bool("list-rules", false, "print the rules in the given files")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	fail := func(err error) int { fmt.Fprintln(stderr, "ocm-lint:", err); return 2 }
	if len(ruleFiles) == 0 || (!*listRules && (fs.NArg() == 0 || (*output != "text" && *output != "json"))) {
		fs.Usage()
		return 2
	}
	rules, warnings, err := ruleset.Load(ruleFiles)
	if err != nil {
		return fail(err)
	}
	for _, w := range warnings {
		fmt.Fprintln(stderr, "ocm-lint: warning:", w)
	}
	loaded := slices.ContainsFunc(rules, func(r lint.Rule) bool { return r.ID() == referencesRule })
	if rules, err = ruleset.Skip(rules, skip); err != nil {
		return fail(err)
	}
	if len(rules) == 0 {
		return fail(errors.New("no rules left to run"))
	}

	if *listRules {
		for _, r := range rules {
			fmt.Fprintf(stdout, "%-40s %-5s %s\n", r.ID(), r.Severity(), r.Description())
		}
		return 0
	}

	if !loaded {
		fmt.Fprintf(stderr, "ocm-lint: warning: %s is not loaded, so missing references may go unreported\n", referencesRule)
	}
	opts := lint.Options{AllowHosts: allow}

	var res lint.Result
	toolErr := false
	visited := map[string]bool{}
	repos := map[string]*lint.Repository{}
	defer func() {
		for _, r := range repos {
			_ = r.Close()
		}
	}()
	for _, arg := range fs.Args() {
		ref, err := lint.ParseRef(arg)
		if err != nil {
			fmt.Fprintln(stderr, "ocm-lint:", err)
			toolErr = true
			continue
		}
		repo, ok := repos[ref.Repo]
		if !ok {
			if repo, err = lint.Open(ctx, ref.Repo); err != nil {
				fmt.Fprintln(stderr, "ocm-lint:", err)
				toolErr = true
				continue
			}
			repos[ref.Repo] = repo
		}
		target, err := repo.Target(ctx, ref.Component, ref.Version, opts)
		if err != nil {
			fmt.Fprintln(stderr, "ocm-lint:", err)
			toolErr = true
			continue
		}
		res.Add(lint.Lint(ctx, target, rules, *recursive, visited))
	}

	write := report.Text
	if *output == "json" {
		write = report.JSON
	}
	if err := write(stdout, res); err != nil {
		return fail(err)
	}

	switch {
	case toolErr || len(res.Errors) > 0:
		return 2
	case res.HasErrors():
		return 1
	}
	return 0
}
