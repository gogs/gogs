package web

import (
	"os"
	"path/filepath"

	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/tool"
)

// maxNoticeBannerSize is the largest notice banner file shown, so a large file
// cannot break pages.
const maxNoticeBannerSize = 1024

// readNoticeBanner returns the HTML of the notice banner that every page shows,
// rendered from the Markdown in "notice/banner.md" under the custom directory.
// It returns an empty string when there is no banner to show.
func readNoticeBanner(customDir string) string {
	fpath := filepath.Join(customDir, "notice", "banner.md")
	fi, err := os.Stat(fpath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Error("Failed to stat notice banner file %q: %v", fpath, err)
		}
		return ""
	}
	if fi.Size() > maxNoticeBannerSize {
		log.Warn("Notice banner file %q is too large [%d > %d]: refusing to render", fpath, fi.Size(), maxNoticeBannerSize)
		return ""
	}

	data, err := os.ReadFile(fpath)
	if err != nil {
		log.Error("Failed to read notice banner file %q: %v", fpath, err)
		return ""
	}
	if !tool.IsTextFile(data) {
		log.Warn("Notice banner file %q does not appear to be a text file: refusing to render", fpath)
		return ""
	}
	return markup.Sanitize(string(markup.RawMarkdown(data, "")))
}
