// Copyright 2019 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package context

import (
	"os"
	"path"

	"github.com/unknwon/com"
	log "gopkg.in/clog.v1"

	"gogs.io/gogs/pkg/markup"
	"gogs.io/gogs/pkg/setting"
	"gogs.io/gogs/pkg/tool"
)

// renderNoticeBanner checks if a notice banner file exists and loads the message to display
// on all pages.
func (c *Context) renderNoticeBanner() {
	fpath := path.Join(setting.CustomPath, "notice", "banner.md")
	if !com.IsExist(fpath) {
		return
	}

	f, err := os.Open(fpath)
	if err != nil {
		log.Error(2, "Failed to open file %q: %v", fpath, err)
		return
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		log.Error(2, "Failed to stat file %q: %v", fpath, err)
		return
	}

	// Limit size to prevent very large messages from breaking pages
	var maxSize int64 = 1024

	if fi.Size() > maxSize { // Refuse to print very long messages
		log.Warn("Notice banner file %q size too large [%d > %d]: refusing to render", fpath, fi.Size(), maxSize)
		return
	}

	buf := make([]byte, maxSize)
	n, err := f.Read(buf)
	if err != nil {
		log.Error(2, "Failed to read file %q: %v", fpath, err)
		return
	}
	buf = buf[:n]

	if !tool.IsTextFile(buf) {
		log.Warn("Notice banner file %q does not appear to be a text file: aborting", fpath)
		return
	}

	c.Data["ServerNotice"] = string(markup.RawMarkdown(buf, ""))
}
