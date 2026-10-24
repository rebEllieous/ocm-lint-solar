# ocm-lint

Lints OCM component versions for delivery: transfer, air-gap, Solar.

```
go install github.com/rebEllieous/ocm-lint-solar/cmd/ocm-lint@latest
ocm-lint --rule air-gap --rule solar --recursive ghcr.io/acme//acme.org/app:1.0.0
```

Rules: `ocm-lint --list-rules`. Exit codes: 0 ok, 1 errors found, 2 tool error.
