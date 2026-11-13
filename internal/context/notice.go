package context

import (
	"os"
	"path/filepath"

	"gogs.io/gogs/internal/applog"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/osx"
	"gogs.io/gogs/internal/tool"
)

// renderNoticeBanner checks if a notice banner file exists and loads the message to display
// on all pages.
func (c *Context) renderNoticeBanner() {
	fpath := filepath.Join(conf.CustomDir(), "notice", "banner.md")
	if !osx.Exist(fpath) {
		return
	}

	f, err := os.Open(fpath)
	if err != nil {
		applog.Logger.Errorf("Failed to open file %q: %v", fpath, err)
		return
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		applog.Logger.Errorf("Failed to stat file %q: %v", fpath, err)
		return
	}

	// Limit size to prevent very large messages from breaking pages
	var maxSize int64 = 1024

	if fi.Size() > maxSize { // Refuse to print very long messages
		applog.Logger.Warnf("Notice banner file %q size too large [%d > %d]: refusing to render", fpath, fi.Size(), maxSize)
		return
	}

	buf := make([]byte, maxSize)
	n, err := f.Read(buf)
	if err != nil {
		applog.Logger.Errorf("Failed to read file %q: %v", fpath, err)
		return
	}
	buf = buf[:n]

	if !tool.IsTextFile(buf) {
		applog.Logger.Warnf("Notice banner file %q does not appear to be a text file: aborting", fpath)
		return
	}

	c.Data["ServerNotice"] = string(markup.RawMarkdown(buf, ""))
}
