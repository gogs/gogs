package templates

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadInjectFile(t *testing.T) {
	t.Run("embedded default is empty", func(t *testing.T) {
		got, err := ReadInjectFile(filepath.Join(t.TempDir(), "templates"), "head.tmpl")
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("custom file takes precedence", func(t *testing.T) {
		customDir := filepath.Join(t.TempDir(), "templates")
		require.NoError(t, os.MkdirAll(filepath.Join(customDir, "inject"), 0700))
		want := `<meta name="custom" content="1">`
		require.NoError(t, os.WriteFile(filepath.Join(customDir, "inject", "head.tmpl"), []byte(want), 0600))

		got, err := ReadInjectFile(customDir, "head.tmpl")
		require.NoError(t, err)
		assert.Equal(t, want, string(got))
	})

	t.Run("missing embedded file errors", func(t *testing.T) {
		_, err := ReadInjectFile(filepath.Join(t.TempDir(), "templates"), "does-not-exist.tmpl")
		assert.Error(t, err)
	})
}
