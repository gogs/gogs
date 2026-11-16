package expr

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// xlangExpressions returns the expressions of GitHub's conformance suite, to
// seed the fuzzers.
func xlangExpressions(f *testing.F) []string {
	files, err := filepath.Glob("testdata/xlang/*.json")
	require.NoError(f, err)
	var out []string
	for _, file := range files {
		data, err := os.ReadFile(file)
		require.NoError(f, err)
		var groups map[string][]xlangCase
		require.NoError(f, json.Unmarshal(data, &groups))
		for _, cases := range groups {
			for _, c := range cases {
				out = append(out, c.Expr)
			}
		}
	}
	return out
}

// checkValue checks that a value is one of the types of expression values,
// recursively.
func checkValue(t *testing.T, v any, depth int) {
	require.Less(t, depth, 1000, "value nests too deep")
	switch v := v.(type) {
	case nil, bool, float64, string:
	case []any:
		for _, x := range v {
			checkValue(t, x, depth+1)
		}
	case *Object:
		for _, k := range v.Keys() {
			x, ok := v.Get(k)
			require.True(t, ok, "key %q of an object", k)
			checkValue(t, x, depth+1)
		}
	default:
		t.Fatalf("evaluation returned a %T", v)
	}
}

// FuzzEval parses and evaluates arbitrary expressions. Neither may panic, an
// error must be an *Error, and a result must be an expression value that
// converts to a string and a number.
func FuzzEval(f *testing.F) {
	for _, src := range xlangExpressions(f) {
		f.Add(src)
	}
	github, err := FromJSON(`{"event": {"commits": [{"id": "a", "files": ["x", "y"]}, {"id": "b", "files": []}],
		"number": 3, "draft": false}, "ref": "refs/heads/main", "run_attempt": "1"}`)
	require.NoError(f, err)
	f.Fuzz(func(t *testing.T, src string) {
		e, err := Parse(src)
		if err != nil {
			var target *Error
			assert.ErrorAs(t, err, &target)
			return
		}
		env := &Env{
			Contexts: map[string]any{"github": github, "matrix": nil, "inputs": NewObject()},
			Funcs:    map[string]Func{"success": func([]any) (any, error) { return true, nil }},
		}
		v, err := e.Eval(env)
		if err != nil {
			var target *Error
			assert.ErrorAs(t, err, &target)
			return
		}
		checkValue(t, v, 0)
		ToString(v)
		ToNumber(v)
		Truthy(v)
	})
}

// FuzzJSON checks fromJSON and toJSON on arbitrary text: fromJSON accepts
// only JSON, and toJSON writes JSON that reads back to the same value.
func FuzzJSON(f *testing.F) {
	for _, s := range []string{
		`{"a": [1, 2.5e3, -0, "x\u00e9\n"], "A": null, "b": {"c": true}}`, `[]`, `"\ud83d\ude00"`,
		`1e400`, `{"a":1,"a":2}`, `[1,]`, ` 7 `, `{"k": "\u2028"}`,
	} {
		f.Add(s)
	}
	for _, src := range xlangExpressions(f) {
		if json.Valid([]byte(src)) {
			f.Add(src)
		}
	}
	f.Fuzz(func(t *testing.T, s string) {
		v, err := FromJSON(s)
		if err != nil {
			return
		}
		require.True(t, json.Valid([]byte(s)), "fromJSON accepted text that is not JSON")
		checkValue(t, v, 0)
		if hasNonFinite(v) {
			return // GitHub writes NaN and infinity unquoted, which is not JSON.
		}
		out, err := ToJSON(v)
		require.NoError(t, err)
		require.True(t, json.Valid([]byte(out)), "toJSON wrote %q", out)
		again, err := FromJSON(out)
		require.NoError(t, err)
		out2, err := ToJSON(again)
		require.NoError(t, err)
		assert.Equal(t, out, out2, "toJSON(fromJSON(toJSON(v))) differs from toJSON(v)")
	})
}

func hasNonFinite(v any) bool {
	switch v := v.(type) {
	case float64:
		return math.IsNaN(v) || math.IsInf(v, 0)
	case []any:
		for _, x := range v {
			if hasNonFinite(x) {
				return true
			}
		}
	case *Object:
		for _, k := range v.Keys() {
			x, _ := v.Get(k)
			if hasNonFinite(x) {
				return true
			}
		}
	}
	return false
}
