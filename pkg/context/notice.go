// Copyright 2019 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package context

import (
	"os"
	"path"
	"strings"

	"github.com/Unknwon/com"
	"github.com/gogs/gogs/pkg/setting"
	"github.com/gogs/gogs/pkg/tool"
	log "gopkg.in/clog.v1"
)

// readServerNotice checks if a notice file exists and loads the message to display
// on all pages.
func readServerNotice() map[string]interface{} {
	fileloc := path.Join(setting.CustomPath, "notice")

	if !com.IsExist(fileloc) {
		return nil
	}

	log.Trace("Found notice file")
	fp, err := os.Open(fileloc)
	if err != nil {
		log.Error(2, "Failed to open notice file %s: %v", fileloc, err)
		return nil
	}
	defer fp.Close()

	finfo, err := fp.Stat()
	if err != nil {
		log.Error(2, "Failed to stat notice file %s: %v", fileloc, err)
		return nil
	}

	// Limit size to prevent very large messages from breaking pages
	var maxSize int64 = 1024

	if finfo.Size() > maxSize { // Refuse to print very long messages
		log.Error(2, "Notice file %s size too large [%d > %d]: refusing to render", fileloc, finfo.Size(), maxSize)
		return nil
	}

	buf := make([]byte, maxSize)
	n, err := fp.Read(buf)
	if err != nil {
		log.Error(2, "Failed to read notice file: %v", err)
		return nil
	}
	buf = buf[:n]

	if !tool.IsTextFile(buf) {
		log.Error(2, "Notice file %s does not appear to be a text file: aborting", fileloc)
		return nil
	}

	noticetext := strings.SplitN(string(buf), "\n", 2)
	return map[string]interface{}{
		"Title":   noticetext[0],
		"Message": noticetext[1],
	}
}
