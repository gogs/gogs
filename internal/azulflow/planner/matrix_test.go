package planner

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/azulflow/expr"
	"gogs.io/gogs/internal/azulflow/template"
)

// expand runs the strategy {"matrix": matrix} of a job named "test", and
// returns the JSON of each combination with its display name.
func expand(t *testing.T, matrix string) ([]string, []string, error) {
	t.Helper()
	m, err := expr.FromJSON(`{"matrix": ` + matrix + `}`)
	require.NoError(t, err)
	ctx := template.NewContext(template.WorkflowSchema(), template.DefaultLimits())
	st, err := convertStrategy(ctx, template.FromValue(m, &template.Token{}), "test")
	if err == nil && len(ctx.Errors()) > 0 {
		err = errors.New(strings.Join(ctx.Errors(), ", "))
	}
	if err != nil {
		return nil, nil, err
	}
	var combos, names []string
	for _, c := range st.configs {
		data, err := json.Marshal(c.matrix)
		require.NoError(t, err)
		combos = append(combos, string(data))
		names = append(names, c.name)
	}
	return combos, names, nil
}

func expandJSON(t *testing.T, matrix string) []string {
	t.Helper()
	combos, _, err := expand(t, matrix)
	require.NoError(t, err)
	return combos
}

func TestExpandMatrix(t *testing.T) {
	// The examples below and their results are from GitHub's documentation
	// on matrix strategies.
	t.Run("product with the first key varying slowest", func(t *testing.T) {
		got := expandJSON(t, `{"version": [10, 12], "os": ["ubuntu", "windows"]}`)
		assert.Equal(t, []string{
			`{"version":10,"os":"ubuntu"}`,
			`{"version":10,"os":"windows"}`,
			`{"version":12,"os":"ubuntu"}`,
			`{"version":12,"os":"windows"}`,
		}, got)
	})

	t.Run("include adds to matching originals or creates combinations", func(t *testing.T) {
		got := expandJSON(t, `{
			"fruit": ["apple", "pear"],
			"animal": ["cat", "dog"],
			"include": [
				{"color": "green"},
				{"color": "pink", "animal": "cat"},
				{"fruit": "apple", "shape": "circle"},
				{"fruit": "banana"},
				{"fruit": "banana", "animal": "cat"}
			]
		}`)
		assert.Equal(t, []string{
			`{"fruit":"apple","animal":"cat","color":"pink","shape":"circle"}`,
			`{"fruit":"apple","animal":"dog","color":"green","shape":"circle"}`,
			`{"fruit":"pear","animal":"cat","color":"pink"}`,
			`{"fruit":"pear","animal":"dog","color":"green"}`,
			`{"fruit":"banana"}`,
			`{"fruit":"banana","animal":"cat"}`,
		}, got)
	})

	t.Run("exclude removes partial matches before include", func(t *testing.T) {
		got := expandJSON(t, `{
			"os": ["macos", "windows"],
			"version": [12, 14, 16],
			"environment": ["staging", "production"],
			"exclude": [
				{"os": "macos", "version": 12, "environment": "production"},
				{"os": "windows", "version": 16}
			],
			"include": [{"os": "windows", "version": 16, "experimental": true}]
		}`)
		assert.Len(t, got, 10)
		assert.NotContains(t, got, `{"os":"macos","version":12,"environment":"production"}`)
		assert.Equal(t, `{"os":"windows","version":16,"experimental":true}`, got[9])
	})

	t.Run("only include", func(t *testing.T) {
		got := expandJSON(t, `{"include": [{"site": "a"}, {"site": "b", "dir": "x"}]}`)
		assert.Equal(t, []string{`{"site":"a"}`, `{"site":"b","dir":"x"}`}, got)
	})

	t.Run("names", func(t *testing.T) {
		_, names, err := expand(t, `{
			"node": [{"version": 20, "lts": true}],
			"os": ["linux", ""],
			"include": [{"os": "linux", "extra": "not named"}, {"os": "mac", "arch": "arm"}]
		}`)
		require.NoError(t, err)
		assert.Equal(t, []string{"test (20, true, linux)", "test (20, true)", "test (mac, arm)"}, names)
	})

	t.Run("errors", func(t *testing.T) {
		for matrix, want := range map[string]string{
			`{"os": []}`:                              "Matrix vector 'os' does not contain any values",
			`{"os": "linux"}`:                         "Unexpected type 'StringToken' encountered while reading 'matrix vector value'. The type 'SequenceToken' was expected.",
			`{"include": "x"}`:                        "Unexpected type 'StringToken' encountered while reading 'matrix includes'. The type 'SequenceToken' was expected.",
			`{"exclude": [{"os": "a"}]}`:              "Matrix must define at least one vector",
			`{"os": ["a"], "exclude": [{"arch": 1}]}`: "Matrix exclude key 'arch' does not match any key within the matrix",
			`{"a": [` + strings.Repeat("1,", 16) + `1], "b": [` + strings.Repeat("1,", 16) + `1]}`: "The matrix produced more than 256 jobs",
		} {
			_, _, err := expand(t, matrix)
			assert.ErrorContains(t, err, want, matrix)
		}
	})

	t.Run("every combination excluded", func(t *testing.T) {
		combos, _, err := expand(t, `{"os": ["a"], "exclude": [{"os": "a"}]}`)
		require.NoError(t, err)
		assert.Empty(t, combos, "the job then runs once without a matrix, as on GitHub")
	})
}
