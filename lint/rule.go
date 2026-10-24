// Package lint runs rules against OCM component versions.
package lint

import (
	"context"
	"fmt"
)

type Severity int

const (
	Warn Severity = iota
	Error
)

func (s Severity) String() string {
	if s == Error {
		return "error"
	}
	return "warn"
}

func (s Severity) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// Finding is one rule violation. Rules set Subject and Message; Run fills in the rest.
type Finding struct {
	Component string   `json:"component"`
	RuleID    string   `json:"rule"`
	Severity  Severity `json:"severity"`
	Subject   string   `json:"subject"`
	Message   string   `json:"message"`
}

// ToolError records a rule that could not run. It never counts as a pass.
type ToolError struct {
	Component string `json:"component"`
	RuleID    string `json:"rule"`
	Message   string `json:"message"`
}

type Rule interface {
	ID() string // "<pack>/<name>"
	Severity() Severity
	Description() string // one line, shown by --list-rules
	Check(ctx context.Context, t *Target) ([]Finding, error)
}

// Func adapts a check function to Rule.
type Func struct {
	RuleID string
	Sev    Severity
	Desc   string
	Fn     func(ctx context.Context, t *Target) ([]Finding, error)
}

func (f Func) ID() string          { return f.RuleID }
func (f Func) Severity() Severity  { return f.Sev }
func (f Func) Description() string { return f.Desc }
func (f Func) Check(ctx context.Context, t *Target) ([]Finding, error) {
	return f.Fn(ctx, t)
}

// F builds a finding with a formatted message.
func F(subject, format string, args ...any) Finding {
	return Finding{Subject: subject, Message: fmt.Sprintf(format, args...)}
}
