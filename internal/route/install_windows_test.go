//go:build windows

package route

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeInstallPath(t *testing.T) {
	t.Run("preserves UNC shares entered with backslashes", func(t *testing.T) {
		assert.Equal(t, `\\imb-nas\repositories`, normalizeInstallPath(`\\imb-nas\repositories`))
	})

	t.Run("normalizes UNC shares entered with forward slashes", func(t *testing.T) {
		assert.Equal(t, `\\imb-nas\repositories`, normalizeInstallPath(`//imb-nas/repositories`))
	})

	t.Run("normalizes drive paths to native separators", func(t *testing.T) {
		assert.Equal(t, `C:\gogs\repositories`, normalizeInstallPath(`C:/gogs/repositories`))
	})
}
