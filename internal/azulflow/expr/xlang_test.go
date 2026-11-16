package expr

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// xlangCase is a case of GitHub's cross-language expression conformance
// suite, copied from github.com/actions/languageservices into testdata/xlang.
type xlangCase struct {
	Expr   string `json:"expr"`
	Result struct {
		Kind  string          `json:"kind"`
		Value json.RawMessage `json:"value"`
	} `json:"result"`
	Err *struct {
		Kind  string `json:"kind"` // "lexing", "parsing" or "evaluation".
		Value string `json:"value"`
	} `json:"err"`
	Contexts json.RawMessage `json:"contexts"`
}

// TestXlang runs the suite the way its TypeScript harness (xlang.test.ts)
// does, including the cases that harness skips, which encode the behavior of
// GitHub's other implementations.
func TestXlang(t *testing.T) {
	files, err := filepath.Glob("testdata/xlang/*.json")
	require.NoError(t, err)
	require.Len(t, files, 28)
	for _, file := range files {
		data, err := os.ReadFile(file)
		require.NoError(t, err)
		var groups map[string][]xlangCase
		require.NoError(t, json.Unmarshal(data, &groups), file)
		base := strings.TrimSuffix(filepath.Base(file), ".json")
		names := make([]string, 0, len(groups))
		for name := range groups {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			for _, c := range groups[name] {
				t.Run(base+"/"+name, func(t *testing.T) {
					runXlangCase(t, c)
				})
			}
		}
	}
}

func runXlangCase(t *testing.T, c xlangCase) {
	contexts := NewObject()
	if len(c.Contexts) > 0 {
		v, err := FromJSON(string(c.Contexts))
		require.NoError(t, err)
		contexts = v.(*Object)
	}
	// The contexts of a case are the only named-values it may use.
	names := append([]string{}, contexts.Keys()...)
	env := &Env{Contexts: map[string]any{}}
	for _, name := range names {
		v, _ := contexts.Get(name)
		env.Contexts[strings.ToLower(name)] = v
	}

	expectError := func(phase string, err error) bool {
		if c.Err == nil || c.Err.Kind != phase {
			require.NoError(t, err, "unexpected error %s", phase)
			return false
		}
		require.Error(t, err, "expected an error %s", phase)
		var exprErr *Error
		require.ErrorAs(t, err, &exprErr)
		// The suite's messages are prefixes of GitHub's.
		assert.True(t, strings.HasPrefix(err.Error(), c.Err.Value), "error %q does not start with %q", err.Error(), c.Err.Value)
		return true
	}

	_, err := lex(c.Expr)
	if expectError("lexing", err) {
		return
	}
	e, err := ParseWith(c.Expr, ParseOptions{Contexts: names})
	if expectError("parsing", err) {
		return
	}
	got, err := e.Eval(env)
	if expectError("evaluation", err) {
		return
	}

	assert.Equal(t, c.Result.Kind, kindName(got))
	wantJSON := "null"
	if len(c.Result.Value) > 0 {
		wantJSON = string(c.Result.Value)
	}
	want, err := FromJSON(wantJSON)
	require.NoError(t, err)
	assert.Equal(t, stringify(want), stringify(got))
}

// stringify encodes a value as JSON.stringify does for the purpose of
// comparing values: -0 is 0, and NaN and infinities are null.
func stringify(v any) string {
	switch v := v.(type) {
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(v)
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return "null"
		}
		if v == 0 {
			return "0"
		}
		return strconv.FormatFloat(v, 'g', -1, 64)
	case string:
		return strconv.Quote(v)
	case *Object:
		parts := make([]string, v.Len())
		for i, k := range v.Keys() {
			value, _ := v.Get(k)
			parts[i] = strconv.Quote(k) + ":" + stringify(value)
		}
		return "{" + strings.Join(parts, ",") + "}"
	}
	arr, _ := asArray(v)
	parts := make([]string, len(arr))
	for i, item := range arr {
		parts[i] = stringify(item)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
