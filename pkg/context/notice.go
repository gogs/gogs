// Copyright 2019 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package context

import (
	"os"
	"path"

	"github.com/Unknwon/com"
	log "gopkg.in/clog.v1"

	"github.com/gogs/gogs/pkg/markup"
	"github.com/gogs/gogs/pkg/setting"
	"github.com/gogs/gogs/pkg/tool"
)

// readServerNotice checks if a notice file exists and loads the message to display
// on all pages.
func readServerNotice() string {
	fpath := path.Join(setting.CustomPath, "notice.md")
	if !com.IsExist(fpath) {
		return ""
	}

	f, err := os.Open(fpath)
	if err != nil {
		log.Error(2, "Failed to open notice file %s: %v", fpath, err)
		return ""
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		log.Error(2, "Failed to stat notice file %s: %v", fpath, err)
		return ""
	}

	// Limit size to prevent very large messages from breaking pages
	var maxSize int64 = 1024

	if fi.Size() > maxSize { // Refuse to print very long messages
		log.Error(2, "Notice file %s size too large [%d > %d]: refusing to render", fpath, fi.Size(), maxSize)
		return ""
	}

	buf := make([]byte, maxSize)
	n, err := f.Read(buf)
	if err != nil {
		log.Error(2, "Failed to read notice file: %v", err)
		return ""
	}
	buf = buf[:n]

	if !tool.IsTextFile(buf) {
		log.Error(2, "Notice file %s does not appear to be a text file: aborting", fpath)
		return ""
	}

	return string(markup.RawMarkdown(buf, ""))
}
