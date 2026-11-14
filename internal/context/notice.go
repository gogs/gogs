package context

import (
	"os"
	"path/filepath"

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
		c.Logger.Error("Failed to open notice banner file", "path", fpath, "error", err)
		return
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		c.Logger.Error("Failed to stat notice banner file", "path", fpath, "error", err)
		return
	}

	// Limit size to prevent very large messages from breaking pages
	var maxSize int64 = 1024

	if fi.Size() > maxSize { // Refuse to print very long messages
		c.Logger.Warn("Refusing to render notice banner file that is too large", "path", fpath, "size", fi.Size(), "maxSize", maxSize)
		return
	}

	buf := make([]byte, maxSize)
	n, err := f.Read(buf)
	if err != nil {
		c.Logger.Error("Failed to read notice banner file", "path", fpath, "error", err)
		return
	}
	buf = buf[:n]

	if !tool.IsTextFile(buf) {
		c.Logger.Warn("Refusing to render notice banner file that does not appear to be text", "path", fpath)
		return
	}

	c.Data["ServerNotice"] = string(markup.RawMarkdown(buf, ""))
}
