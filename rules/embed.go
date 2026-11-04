// Package rules holds the rule files built into ocm-lint. A file's name without
// ".yaml" is its alias: rules/solar.yaml is "--rule=@solar". Adding a file here and
// rebuilding adds an alias.
package rules

import "embed"

//go:embed *.yaml
var Files embed.FS
