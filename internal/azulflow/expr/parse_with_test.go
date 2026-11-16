package expr

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAllowed(t *testing.T) {
	opts := ParseAllowed([]string{"github", "inputs", "hashFiles(1,255)", "success(0,MAX)"})
	assert.Equal(t, []string{"github", "inputs"}, opts.Contexts)
	assert.Equal(t, []FuncSig{{Name: "hashFiles", Min: 1, Max: 255}, {Name: "success", Min: 0, Max: -1}}, opts.Funcs)

	// A schema that allows no contexts allows no named-values.
	assert.Equal(t, []string{}, ParseAllowed(nil).Contexts)
}

func TestParseWith(t *testing.T) {
	opts := ParseAllowed([]string{"github", "hashFiles(1,255)", "success(0,MAX)"})
	for _, src := range []string{"GitHub.ref", "HASHFILES('a') && success(1, 2, 3)", "case(true, toJSON(github), '')"} {
		_, err := ParseWith(src, opts)
		assert.NoError(t, err, src)
	}

	tests := []struct {
		src     string
		wantMsg string
		wantPos int
	}{
		{"vars.x", "Unrecognized named-value: 'vars'", 1},
		{"github.ref == 'é😊' || always()", "Unrecognized function: 'always'", 24},
		{"1 && hashFiles()", "Too few parameters supplied: 'hashFiles'", 6},
		{"format()", "Too few parameters supplied: 'format'", 1},
	}
	for _, test := range tests {
		t.Run(test.src, func(t *testing.T) {
			_, err := ParseWith(test.src, opts)
			var exprErr *Error
			require.ErrorAs(t, err, &exprErr)
			assert.Equal(t, test.wantMsg, exprErr.Msg)
			assert.Equal(t, test.wantPos, exprErr.Pos)
		})
	}

	// Nil contexts allow any named-value.
	_, err := ParseWith("anything.at.all", ParseOptions{})
	assert.NoError(t, err)
}

// TestGitHubSemantics covers behavior of GitHub's C# implementation that the
// conformance suite does not.
func TestGitHubSemantics(t *testing.T) {
	assert.Equal(t, "1E+15", ToString(1e15))
	assert.Equal(t, "1.23456789012346", ToString(1.2345678901234567))
	assert.Equal(t, "1E-05", ToString(0.00001))
	assert.Equal(t, "0.0001", ToString(0.0001))
	assert.Equal(t, -1.0, ToNumber("0xFFFFFFFF"))
	assert.True(t, math.IsNaN(ToNumber("0X1")))

	tests := []struct {
		src  string
		want any
	}{
		// Functions evaluate arguments only when they need them.
		{"case(true, 'a', fromJSON('{'))", "a"},
		{"join(fromJSON('[1]'), fromJSON('{'))", "1"},
		{"format('{1}', fromJSON('{'), 'b')", "b"},
		{"'a''b''c'", "a'b'c"},
		{"toJSON('\u2028')", `"\u2028"`},
	}
	for _, test := range tests {
		t.Run(test.src, func(t *testing.T) {
			e, err := Parse(test.src)
			require.NoError(t, err)
			got, err := e.Eval(&Env{})
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}
