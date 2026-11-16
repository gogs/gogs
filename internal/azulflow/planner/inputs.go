package planner

import (
	"math"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/azulflow/expr"
	"gogs.io/gogs/internal/azulflow/model"
)

// dispatchInputs checks the workflow_dispatch inputs the user gave against
// their declarations, applies defaults, and converts values to the declared
// types: booleans and numbers stay booleans and numbers in the inputs
// context.
func dispatchInputs(w *model.Workflow, given map[string]any) (*expr.Object, error) {
	values, _ := expr.Normalize(given).(*expr.Object)
	if values == nil {
		values = expr.NewObject()
	}
	ev := w.Events.Get("workflow_dispatch")
	if ev == nil {
		return values, nil
	}
	var decls []*model.Input
	if ev.WorkflowDispatch != nil {
		decls = ev.WorkflowDispatch.Inputs
	}
	types := map[string]string{}
	for _, t := range w.InputTypes {
		types[strings.ToLower(t.Name)] = t.Type
	}

	for _, key := range values.Keys() {
		if !slices.ContainsFunc(decls, func(in *model.Input) bool { return strings.EqualFold(in.Name, key) }) {
			return nil, errors.Newf("input %q is not defined", key)
		}
	}
	inputs := expr.NewObject()
	for _, in := range decls {
		typ := types[strings.ToLower(in.Name)]
		v, ok := values.Get(in.Name)
		if !ok {
			if in.DefaultValue == nil {
				if in.Required {
					return nil, errors.Newf("input %q is required", in.Name)
				}
			} else {
				v = in.DefaultValue.Value()
			}
		}
		v, err := coerceInput(in, typ, v)
		if err != nil {
			return nil, err
		}
		inputs.Set(in.Name, v)
	}
	return inputs, nil
}

func coerceInput(in *model.Input, typ string, v any) (any, error) {
	switch typ {
	case "boolean":
		switch x := v.(type) {
		case nil:
			return false, nil
		case bool:
			return x, nil
		case string:
			switch strings.ToLower(x) {
			case "true":
				return true, nil
			case "false":
				return false, nil
			}
		}
	case "number":
		switch x := v.(type) {
		case nil:
			return 0.0, nil
		case float64:
			return x, nil
		case string:
			if f := expr.ToNumber(x); !math.IsNaN(f) && strings.TrimSpace(x) != "" {
				return f, nil
			}
		}
	default:
		switch v.(type) {
		case []any, *expr.Object:
		default:
			s := expr.ToString(v)
			if typ == "choice" && v != nil && !slices.Contains(in.Options, s) {
				return nil, errors.Newf("input %q must be one of %s, not %q", in.Name, strings.Join(in.Options, ", "), s)
			}
			return s, nil
		}
	}
	if typ == "" {
		typ = "string"
	}
	return nil, errors.Newf("input %q must be a %s, not %q", in.Name, typ, expr.ToString(v))
}
