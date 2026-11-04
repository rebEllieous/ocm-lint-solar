# ocm-lint

Lints OCM component versions for delivery: transfer, air-gap, Solar.

```
go install github.com/rebEllieous/ocm-lint-solar/cmd/ocm-lint@latest
ocm-lint --rule rules/core.yaml --rule rules/solar.yaml --rule ./cluster.yaml --recursive ghcr.io/acme//acme.org/app:1.0.0
```

Rules live in YAML files, with checks written in [CEL](https://cel.dev). This repo's are in `rules/`; you reference them by path. At least one `--rule` is required, and `core` only runs if you pass it.

A rule replaces one with the same ID from an earlier file. Use your own pack for new rules: an ID in an earlier file's pack that matches nothing gets a warning, because it looks like a mistyped override.

```yaml
rules:
  - id: solar/single-helm-chart       # replaces the rule from solar.yaml
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

- `schemaErrors()`, `identityErrors()`, `unresolvedReferences()`, `unresolvedTransitiveReferences()`: lists of `{subject, message}`. Use them in `forEach`, as `rules/core.yaml` does, so a registry failure can't be skipped over by `||` or `&&`.
- `externalReason(imageRef)`: why the reference is outside the repository, or `""` when it is inside.

List rules: `ocm-lint --rule <file>... --list-rules`. Exit codes: 0 ok, 1 errors found, 2 tool error, including a rule file that fails to load.
