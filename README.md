# ocm-lint

Lints OCM component versions for delivery: transfer, air-gap, Solar.

```
ocm-lint --rule=@core --rule=@solar --rule=./cluster.yaml --recursive ghcr.io/acme//acme.org/app:1.0.0
```

## Install

Download a binary from [releases](https://github.com/rebEllieous/ocm-lint-solar/releases), or:

```
go install github.com/rebEllieous/ocm-lint-solar/cmd/ocm-lint@latest
```

To check a release file before using it, such as before carrying it into an air-gapped environment:

```
gh attestation verify <file> --repo rebEllieous/ocm-lint-solar
```

## Rules

Rules live in YAML files, with checks written in [CEL](https://cel.dev). At least one `--rule` is required, and `core` only runs if you pass it. `--rule` takes:

| Form | Meaning |
|---|---|
| `@core`, `@air-gap`, `@solar` | A rule file built into the binary, always matching its version. Never uses the network |
| `http://...`, `https://...` | Downloaded at the start of each run. Links to GitHub must be raw file links, not the page showing the file |
| Anything else | A local path. Use `./@name` for a file whose name starts with `@` |

In PowerShell, write `--rule=@solar`: an unquoted `--rule @solar` doesn't work, because `@` means splatting there.

A URL is trusted like a local file, so whoever controls it decides what gets checked. If it can't be loaded, the run stops with exit 2.

To build in your own rules, add `rules/acme.yaml` and run `go build ./cmd/ocm-lint`. `--rule=@acme` then works.

A rule replaces one with the same ID from an earlier file. Use your own pack for new rules: an ID in an earlier file's pack that matches nothing gets a warning, because it looks like a mistyped override.

```yaml
rules:
  - id: solar/single-helm-chart       # replaces the rule from @solar
    severity: error
    description: at most three helmChart resources
    check: component.resources.filter(r, r.type == "helmChart").size() <= 3
    message: '"more than three helm charts"'

  - id: acme/charts-from-our-registry # new rule
    severity: warn
    description: helm charts come from our registry
    forEach: component.resources.filter(r, r.type == "helmChart")
    check: item.access.imageReference.startsWith("registry.acme.org/")
    message: '"chart is not in registry.acme.org: " + item.access.imageReference'
```

`check` is true when the rule passes. `message`, `forEach` and the optional `subject` are CEL too. With `forEach`, `item` is the current element, with one finding per failing element. `subject` defaults to `"component"`, or `"resource " + item.name` with `forEach` over resources; other loops must set it. A misspelled field or invalid rule stops the run before linting starts.

Expressions can read:

- `component`: `name`, `version`, `labels`, `resources`, `sources`, `references`
- a resource: `name`, `version`, `type`, `relation`, `labels`, `hasDigest`, `access`, `effectiveAccess`
- a source: `name`, `version`, `type`, `labels`, `access`, `effectiveAccess`
- a reference: `name`, `component`, `version`, `labels`
- a label: `name`, `value` (any JSON)
- `access`: `type` (without `/vN`), `imageReference`, `reference`, `mediaType`, `referenceName`, `isOCI`, `isLocalBlob`, `isRelative`, `holdsOCIContent`
- `effectiveAccess`: the `globalAccess` when there is one, otherwise `access`
- `location`: `spec`, `host`, `namespace`
- `options`: `allowHosts`

Functions:

- `schemaErrors()`, `identityErrors()`, `unresolvedReferences()`, `unresolvedTransitiveReferences()`: lists of `{subject, message}`. Use them in `forEach`, as `@core` does, so a registry failure can't be skipped over by `||` or `&&`.
- `externalReason(imageRef)`: why the reference is outside the repository, or `""` when it is inside.

List rules: `ocm-lint --rule=<source>... --list-rules`. Exit codes: 0 ok, 1 errors found, 2 tool error, including a rule file that fails to load.

## Releases

Merging the release PR that release-please keeps open creates the version and a draft release. The workflow then adds the binaries and publishes it. If it fails before publishing, re-run the failed job. Commits reaching `main`, including squash-merge titles, must be [conventional commits](https://www.conventionalcommits.org), or they won't count towards the version or changelog.
