package expr

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testEnv(t *testing.T) *Env {
	event, err := FromJSON(`{"commits": [{"message": "a", "author": {"name": "Ann"}}, {"message": "b", "author": {"name": "Bob"}}], "number": 7}`)
	require.NoError(t, err)
	github := NewObject()
	github.Set("event_name", "push")
	github.Set("ref", "refs/heads/main")
	github.Set("event", event)
	matrix := NewObject()
	matrix.Set("os", "ubuntu-latest")
	matrix.Set("node", 20.0)
	return &Env{Contexts: map[string]any{"github": github, "matrix": matrix}}
}

func TestEval(t *testing.T) {
	tests := []struct {
		src  string
		want any
	}{
		// Strings compare without case, and values of different types compare
		// as numbers.
		{"'ABC' == 'abc'", true},
		{"github.ref == 'REFS/HEADS/MAIN'", true},
		{"'1' == 1", true},
		{"'' == 0", true},
		{"null == 0", true},
		{"true == 1", true},
		{"'abc' == 0", false},
		{"NaN == NaN", false},
		{"'a' < 'B'", true},
		{"2 > '10'", false},
		{"'x' < 1", false},
		// Logical operators return an operand, not a boolean.
		{"'' || 'fallback'", "fallback"},
		{"'first' && 'second'", "second"},
		{"0 && unknown_function_is_not_called()", 0.0},
		{"!''", true},
		{"!(1 == 1) || matrix.node >= 20", true},
		// Property access ignores case, and missing properties are null.
		{"GitHub.Event_Name", "push"},
		{"github.missing.deeper", nil},
		{"matrix['os']", "ubuntu-latest"},
		{"github.event.commits[1].message", "b"},
		{"github.event.commits[5]", nil},
		// Object filters apply property access to every element.
		{"contains(github.event.commits.*.author.name, 'bob')", true},
		{"join(github.event.commits.*.message, '+')", "a+b"},
		{"join(github.event.commits.*.message)", "a,b"},
		// Numbers.
		{"0xff", 255.0},
		{"-1.5e1", -15.0},
		{"1 == 1.0", true},
		// Functions.
		{"startsWith(github.ref, 'refs/HEADS/')", true},
		{"endsWith('file.YML', '.yml')", true},
		{"contains('Hello', 'ELL')", true},
		{"format('{0} {{literal}} {1}', 'a', 2)", "a {literal} 2"},
		{"toJSON(matrix)", "{\n  \"os\": \"ubuntu-latest\",\n  \"node\": 20\n}"},
		{"fromJSON('[1, \"two\"]')[1]", "two"},
		{"fromJSON('{\"b\": 1, \"a\": 2}').A", 2.0},
		{"toJSON(fromJSON('{\"b\": 1, \"a\": 2}'))", "{\n  \"b\": 1,\n  \"a\": 2\n}"},
		{"'it''s'", "it's"},
	}
	for _, test := range tests {
		t.Run(test.src, func(t *testing.T) {
			e, err := Parse(test.src)
			require.NoError(t, err)
			got, err := e.Eval(testEnv(t))
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestParseErrors(t *testing.T) {
	for _, src := range []string{"1 +", "a.", "'open", "f(1,", "a == == b", "(1", "a[1"} {
		t.Run(src, func(t *testing.T) {
			_, err := Parse(src)
			assert.Error(t, err)
		})
	}
}

func TestEvalErrors(t *testing.T) {
	for _, src := range []string{"secrets.token", "hashFiles('x')", "format('{1}', 'a')", "fromJSON('{')"} {
		t.Run(src, func(t *testing.T) {
			e, err := Parse(src)
			require.NoError(t, err)
			_, err = e.Eval(testEnv(t))
			assert.Error(t, err)
		})
	}
}

func TestToString(t *testing.T) {
	assert.Equal(t, "3", ToString(3.0))
	assert.Equal(t, "0.5", ToString(0.5))
	assert.Equal(t, "Infinity", ToString(math.Inf(1)))
	assert.Equal(t, "", ToString(nil))
	assert.Equal(t, "Object", ToString(NewObject()))
}

func TestNamedValues(t *testing.T) {
	e, err := Parse("needs.a.outputs.x || fromJSON(inputs.list)[0] || success() && Needs.b")
	require.NoError(t, err)
	assert.Equal(t, []string{"needs", "inputs"}, e.NamedValues())
	assert.Equal(t, []string{"fromjson", "success"}, e.Functions())
}
