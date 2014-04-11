// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package v1

import (
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/middleware"
)

func SearchCommits(ctx *middleware.Context) {
	userName := ctx.Query("username")
	repoName := ctx.Query("reponame")
	branch := ctx.Query("branch")
	keyword := ctx.Query("q")
	if len(keyword) == 0 {
		ctx.Render.JSON(404, nil)
		return
	}

	commits, err := models.SearchCommits(models.RepoPath(userName, repoName), branch, keyword)
	if err != nil {
		ctx.Render.JSON(200, map[string]interface{}{"ok": false})
		return
	}

	ctx.Render.JSON(200, map[string]interface{}{
		"ok":      true,
		"commits": commits,
	})
}
