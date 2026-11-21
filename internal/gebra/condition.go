package gebra

import (
	"encoding/base64"
	"encoding/json"
	"math"
	"net"
	"slices"
	"strconv"
	"time"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/common/types"
	"cel.dev/cel-go/common/types/ref"
	"github.com/cockroachdb/errors"
)

type compiledCondition struct {
	name    string
	params  map[string]*ConditionParamTypeRef
	program cel.Program
}

func celType(name string, p *ConditionParamTypeRef) (*cel.Type, error) {
	generic := func() (*cel.Type, error) {
		if len(p.GenericTypes) != 1 {
			return nil, invalidf("condition parameter %q: %s needs exactly one generic type", name, p.TypeName)
		}
		return celType(name, p.GenericTypes[0])
	}
	switch p.TypeName {
	case "TYPE_NAME_INT":
		return cel.IntType, nil
	case "TYPE_NAME_UINT":
		return cel.UintType, nil
	case "TYPE_NAME_DOUBLE":
		return cel.DoubleType, nil
	case "TYPE_NAME_BOOL":
		return cel.BoolType, nil
	case "TYPE_NAME_BYTES":
		return cel.BytesType, nil
	case "TYPE_NAME_STRING", "TYPE_NAME_IPADDRESS":
		return cel.StringType, nil
	case "TYPE_NAME_DURATION":
		return cel.DurationType, nil
	case "TYPE_NAME_TIMESTAMP":
		return cel.TimestampType, nil
	case "TYPE_NAME_ANY":
		return cel.DynType, nil
	case "TYPE_NAME_LIST":
		elem, err := generic()
		if err != nil {
			return nil, err
		}
		return cel.ListType(elem), nil
	case "TYPE_NAME_MAP":
		elem, err := generic()
		if err != nil {
			return nil, err
		}
		return cel.MapType(cel.StringType, elem), nil
	default:
		return nil, invalidf("condition parameter %q has unknown type %q", name, p.TypeName)
	}
}

// inCIDR implements ipaddress.in_cidr(cidr). IP addresses are strings that
// were validated when the context was converted.
var inCIDR = cel.Function("in_cidr",
	cel.MemberOverload("ipaddress_in_cidr", []*cel.Type{cel.StringType, cel.StringType}, cel.BoolType,
		cel.BinaryBinding(func(lhs, rhs ref.Val) ref.Val {
			ip := net.ParseIP(string(lhs.(types.String)))
			_, network, err := net.ParseCIDR(string(rhs.(types.String)))
			if ip == nil || err != nil {
				return types.NewErr("in_cidr: invalid IP address or CIDR")
			}
			return types.Bool(network.Contains(ip))
		}),
	),
)

func compileCondition(name string, c *Condition, costLimit uint64) (*compiledCondition, error) {
	if !validName(name) {
		return nil, invalidf("invalid condition name %q", name)
	}
	opts := []cel.EnvOption{inCIDR}
	for pname, p := range c.Parameters {
		t, err := celType(pname, p)
		if err != nil {
			return nil, err
		}
		opts = append(opts, cel.Variable(pname, t))
	}
	env, err := cel.NewEnv(opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "condition %q: create environment", name)
	}
	ast, iss := env.Compile(c.Expression)
	if iss.Err() != nil {
		return nil, invalidf("condition %q: %v", name, iss.Err())
	}
	if !ast.OutputType().IsExactType(cel.BoolType) && !ast.OutputType().IsExactType(cel.DynType) {
		return nil, invalidf("condition %q must evaluate to bool, not %s", name, ast.OutputType())
	}
	prg, err := env.Program(ast, cel.CostLimit(costLimit))
	if err != nil {
		return nil, invalidf("condition %q: %v", name, err)
	}
	return &compiledCondition{name: name, params: c.Parameters, program: prg}, nil
}

// validateContext reports whether every value in ctx is a declared parameter
// of the right type.
func (c *compiledCondition) validateContext(ctx map[string]any) error {
	for k, v := range ctx {
		p, ok := c.params[k]
		if !ok {
			return invalidf("condition %q has no parameter %q", c.name, k)
		}
		if _, err := convertParam(k, p, v); err != nil {
			return err
		}
	}
	return nil
}

// eval evaluates the condition. Values stored with the tuple take precedence
// over values from the request.
func (c *compiledCondition) eval(tupleCtx, requestCtx map[string]any) (bool, error) {
	vars := make(map[string]any, len(c.params))
	var missing []string
	for name, p := range c.params {
		raw, ok := tupleCtx[name]
		if !ok {
			raw, ok = requestCtx[name]
		}
		if !ok {
			missing = append(missing, name)
			continue
		}
		v, err := convertParam(name, p, raw)
		if err != nil {
			return false, err
		}
		vars[name] = v
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		return false, errors.Wrapf(ErrMissingContext, "condition %q needs %v", c.name, missing)
	}
	out, _, err := c.program.Eval(vars)
	if err != nil {
		return false, invalidf("evaluate condition %q: %v", c.name, err)
	}
	b, ok := out.Value().(bool)
	if !ok {
		return false, invalidf("condition %q did not evaluate to bool", c.name)
	}
	return b, nil
}

// convertParam converts a JSON value to the Go value CEL expects for the
// parameter type.
func convertParam(name string, p *ConditionParamTypeRef, v any) (any, error) {
	bad := func() (any, error) {
		return nil, invalidf("condition parameter %q: %v is not a valid %s", name, v, p.TypeName)
	}
	switch p.TypeName {
	case "TYPE_NAME_INT":
		f, ok := number(v)
		if !ok || f != math.Trunc(f) {
			return bad()
		}
		return int64(f), nil
	case "TYPE_NAME_UINT":
		f, ok := number(v)
		if !ok || f < 0 || f != math.Trunc(f) {
			return bad()
		}
		return uint64(f), nil
	case "TYPE_NAME_DOUBLE":
		f, ok := number(v)
		if !ok {
			return bad()
		}
		return f, nil
	case "TYPE_NAME_BOOL":
		switch b := v.(type) {
		case bool:
			return b, nil
		case string:
			parsed, err := strconv.ParseBool(b)
			if err != nil {
				return bad()
			}
			return parsed, nil
		}
		return bad()
	case "TYPE_NAME_BYTES":
		s, ok := v.(string)
		if !ok {
			return bad()
		}
		if b, err := base64.StdEncoding.DecodeString(s); err == nil {
			return b, nil
		}
		return []byte(s), nil
	case "TYPE_NAME_STRING":
		s, ok := v.(string)
		if !ok {
			return bad()
		}
		return s, nil
	case "TYPE_NAME_IPADDRESS":
		s, ok := v.(string)
		if !ok || net.ParseIP(s) == nil {
			return bad()
		}
		return s, nil
	case "TYPE_NAME_DURATION":
		s, ok := v.(string)
		if !ok {
			return bad()
		}
		d, err := time.ParseDuration(s)
		if err != nil {
			return bad()
		}
		return d, nil
	case "TYPE_NAME_TIMESTAMP":
		s, ok := v.(string)
		if !ok {
			return bad()
		}
		ts, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			return bad()
		}
		return ts, nil
	case "TYPE_NAME_ANY":
		return v, nil
	case "TYPE_NAME_LIST":
		list, ok := v.([]any)
		if !ok || len(p.GenericTypes) != 1 {
			return bad()
		}
		out := make([]any, len(list))
		for i, e := range list {
			c, err := convertParam(name, p.GenericTypes[0], e)
			if err != nil {
				return nil, err
			}
			out[i] = c
		}
		return out, nil
	case "TYPE_NAME_MAP":
		m, ok := v.(map[string]any)
		if !ok || len(p.GenericTypes) != 1 {
			return bad()
		}
		out := make(map[string]any, len(m))
		for k, e := range m {
			c, err := convertParam(name, p.GenericTypes[0], e)
			if err != nil {
				return nil, err
			}
			out[k] = c
		}
		return out, nil
	}
	return bad()
}

func number(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	}
	return 0, false
}
