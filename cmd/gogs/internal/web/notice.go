package web

import (
	"os"
	"path/filepath"

	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/mimex"
)

// maxNoticeBannerSize is the largest notice banner file shown, so a large file
// cannot break pages.
const maxNoticeBannerSize = 1024

// readNoticeBanner returns the HTML of the notice banner that every page shows,
// rendered from the Markdown in "notice/banner.md" under the custom directory.
// It returns an empty string when there is no banner to show.
func readNoticeBanner(logger *logx.Logger, customDir string) string {
	fpath := filepath.Join(customDir, "notice", "banner.md")
	fi, err := os.Stat(fpath)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Error("Failed to stat notice banner file", "path", fpath, "error", err)
		}
		return ""
	}
	if fi.Size() > maxNoticeBannerSize {
		logger.Warn("Notice banner file is too large to render", "path", fpath, "size", fi.Size(), "maxSize", maxNoticeBannerSize)
		return ""
	}

	data, err := os.ReadFile(fpath)
	if err != nil {
		logger.Error("Failed to read notice banner file", "path", fpath, "error", err)
		return ""
	}
	if !mimex.IsText(data) {
		logger.Warn("Notice banner file is not a text file", "path", fpath)
		return ""
	}
	return markup.Sanitize(string(markup.RawMarkdown(data, "")))
}
