package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadNoticeBanner(t *testing.T) {
	writeBanner := func(t *testing.T, content string) string {
		t.Helper()
		customDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(customDir, "notice"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(customDir, "notice", "banner.md"), []byte(content), 0o600))
		return customDir
	}

	t.Run("no banner file", func(t *testing.T) {
		assert.Empty(t, readNoticeBanner(t.TempDir()))
	})

	t.Run("renders Markdown without scripts", func(t *testing.T) {
		got := readNoticeBanner(writeBanner(t, "Maintenance on **Sunday**, see [status](https://status.example.com).<script>alert(1)</script>"))
		assert.Contains(t, got, "<strong>Sunday</strong>")
		assert.Contains(t, got, `href="https://status.example.com"`)
		assert.NotContains(t, got, "<script")
	})

	t.Run("largest file shown", func(t *testing.T) {
		assert.NotEmpty(t, readNoticeBanner(writeBanner(t, strings.Repeat("a", maxNoticeBannerSize))))
	})

	t.Run("file too large", func(t *testing.T) {
		assert.Empty(t, readNoticeBanner(writeBanner(t, strings.Repeat("a", maxNoticeBannerSize+1))))
	})
}
