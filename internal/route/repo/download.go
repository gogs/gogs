// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"fmt"
	"net/http"
	"path"

	"github.com/gogs/git-module"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/gitutil"
	"gogs.io/gogs/internal/tool"
)

func serveData(c *context.Context, name string, data []byte) error {
	commit, err := c.Repo.Commit.CommitByPath(git.CommitByRevisionOptions{Path: c.Repo.TreePath})
	if err != nil {
		return fmt.Errorf("get commit by path %q: %v", c.Repo.TreePath, err)
	}
	c.Resp.Header().Set("Last-Modified", commit.Committer.When.Format(http.TimeFormat))

	if !tool.IsTextFile(data) {
		if !tool.IsImageFile(data) {
			c.Resp.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
			c.Resp.Header().Set("Content-Transfer-Encoding", "binary")
		}
	} else if !conf.Repository.EnableRawFileRenderMode || !c.QueryBool("render") {
		c.Resp.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}

	if _, err := c.Resp.Write(data); err != nil {
		return fmt.Errorf("write buffer to response: %v", err)
	}
	return nil
}

func ServeBlob(c *context.Context, blob *git.Blob) error {
	p, err := blob.Bytes()
	if err != nil {
		return err
	}

	return serveData(c, path.Base(c.Repo.TreePath), p)
}

func SingleDownload(c *context.Context) {
	blob, err := c.Repo.Commit.Blob(c.Repo.TreePath)
	if err != nil {
		c.NotFoundOrError(gitutil.NewError(err), "get blob")
		return
	}

	if err = ServeBlob(c, blob); err != nil {
		c.Error(err, "serve blob")
		return
	}
}
