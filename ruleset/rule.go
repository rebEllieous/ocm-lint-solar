package ruleset

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/common/types"
	"cel.dev/cel-go/common/types/ref"
	"cel.dev/cel-go/common/types/traits"
	"cel.dev/cel-go/ext"

	"github.com/rebEllieous/ocm-lint-solar/lint"
)

const (
	componentType = "ruleset.Component"
	resourceType  = "ruleset.Resource"
	locationType  = "ruleset.Location"
	optionsType   = "ruleset.Options"
)

func newEnv() (*cel.Env, error) {
	return cel.NewEnv(append(functionDecls(),
		ext.NativeTypes(ext.ParseStructTags(true),
			reflect.TypeFor[Component](), reflect.TypeFor[Location](), reflect.TypeFor[Options](), reflect.TypeFor[Problem]()),
		ext.Bindings(),
		ext.Strings(),
		cel.Variable("component", cel.ObjectType(componentType)),
		cel.Variable("location", cel.ObjectType(locationType)),
		cel.Variable("options", cel.ObjectType(optionsType)),
	)...)
}

// ruleSpec is one rule in a rule file.
type ruleSpec struct {
	ID          string `json:"id"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
	ForEach     string `json:"forEach"`
	Check       string `json:"check"`
	Subject     string `json:"subject"`
	Message     string `json:"message"`
}

// rule is a compiled ruleSpec.
type rule struct {
	id, desc                         string
	sev                              lint.Severity
	env                              *cel.Env
	forEach, check, subject, message *cel.Ast
}

func (r *rule) ID() string              { return r.id }
func (r *rule) Severity() lint.Severity { return r.sev }
func (r *rule) Description() string     { return r.desc }

func compile(env *cel.Env, s ruleSpec) (*rule, error) {
	pack, name, ok := strings.Cut(s.ID, "/")
	if !ok || pack == "" || name == "" {
		return nil, fmt.Errorf("id %q is not <pack>/<name>", s.ID)
	}
	r := &rule{id: s.ID, desc: s.Description, env: env}
	switch s.Severity {
	case "error":
		r.sev = lint.Error
	case "warn":
		r.sev = lint.Warn
	default:
		return nil, fmt.Errorf("severity %q is not error or warn", s.Severity)
	}
	switch {
	case s.Description == "":
		return nil, errors.New("description is required")
	case s.Check == "":
		return nil, errors.New("check is required")
	case s.Message == "":
		return nil, errors.New("message is required")
	}

	subject := s.Subject
	if subject == "" {
		subject = `"component"`
	}
	if s.ForEach != "" {
		ast, iss := env.Compile(s.ForEach)
		if iss.Err() != nil {
			return nil, fmt.Errorf("forEach: %w", iss.Err())
		}
		if ast.OutputType().Kind() != cel.ListKind {
			return nil, fmt.Errorf("forEach is %s, want a list", ast.OutputType())
		}
		r.forEach = ast
		elem := ast.OutputType().Parameters()[0]
		var err error
		if r.env, err = env.Extend(cel.Variable("item", elem)); err != nil {
			return nil, fmt.Errorf("forEach: %w", err)
		}
		if s.Subject == "" {
			if elem.TypeName() != resourceType {
				return nil, fmt.Errorf("forEach over %s needs a subject", elem)
			}
			subject = `"resource " + item.name`
		}
	}

	var err error
	if r.check, err = expr(r.env, "check", s.Check, cel.BoolType); err != nil {
		return nil, err
	}
	if r.subject, err = expr(r.env, "subject", subject, cel.StringType); err != nil {
		return nil, err
	}
	if r.message, err = expr(r.env, "message", s.Message, cel.StringType); err != nil {
		return nil, err
	}
	return r, nil
}

// expr compiles an expression that must have type want. Dyn is checked at runtime.
func expr(env *cel.Env, field, src string, want *cel.Type) (*cel.Ast, error) {
	ast, iss := env.Compile(src)
	if iss.Err() != nil {
		return nil, fmt.Errorf("%s: %w", field, iss.Err())
	}
	if got := ast.OutputType(); got.Kind() != cel.DynKind && !got.IsExactType(want) {
		return nil, fmt.Errorf("%s is %s, want %s", field, got, want)
	}
	return ast, nil
}

func (r *rule) Check(ctx context.Context, t *lint.Target) ([]lint.Finding, error) {
	vars, err := newInput(t)
	if err != nil {
		return nil, err
	}
	fns := bindings(ctx, t, r.env.CELTypeAdapter())
	if r.forEach == nil {
		f, err := r.checkOne(fns, vars)
		if err != nil || f == nil {
			return nil, err
		}
		return []lint.Finding{*f}, nil
	}

	list, err := r.eval(fns, r.forEach, vars)
	if err != nil {
		return nil, fmt.Errorf("forEach: %w", err)
	}
	lister, ok := list.(traits.Lister)
	if !ok {
		return nil, fmt.Errorf("forEach evaluated to %s, want a list", list.Type())
	}
	var fs []lint.Finding
	for it := lister.Iterator(); it.HasNext() == types.True; {
		vars["item"] = it.Next()
		f, err := r.checkOne(fns, vars)
		if err != nil {
			return nil, err
		}
		if f != nil {
			fs = append(fs, *f)
		}
	}
	return fs, nil
}

// checkOne returns a finding when check is false.
func (r *rule) checkOne(fns cel.ProgramOption, vars map[string]any) (*lint.Finding, error) {
	v, err := r.eval(fns, r.check, vars)
	if err != nil {
		return nil, fmt.Errorf("check: %w", err)
	}
	pass, ok := v.Value().(bool)
	if !ok {
		return nil, fmt.Errorf("check evaluated to %s, want bool", v.Type())
	}
	if pass {
		return nil, nil
	}
	subject, err := r.evalString(fns, "subject", r.subject, vars)
	if err != nil {
		return nil, err
	}
	message, err := r.evalString(fns, "message", r.message, vars)
	if err != nil {
		return nil, err
	}
	return &lint.Finding{Subject: subject, Message: message}, nil
}

func (r *rule) evalString(fns cel.ProgramOption, field string, ast *cel.Ast, vars map[string]any) (string, error) {
	v, err := r.eval(fns, ast, vars)
	if err != nil {
		return "", fmt.Errorf("%s: %w", field, err)
	}
	s, ok := v.Value().(string)
	if !ok {
		return "", fmt.Errorf("%s evaluated to %s, want string", field, v.Type())
	}
	return s, nil
}

func (r *rule) eval(fns cel.ProgramOption, ast *cel.Ast, vars map[string]any) (ref.Val, error) {
	prg, err := r.env.Program(ast, fns)
	if err != nil {
		return nil, err
	}
	v, _, err := prg.Eval(vars)
	return v, err
}
