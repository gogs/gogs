// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"fmt"
	"net/http"

	"github.com/martini-contrib/render"

	"github.com/gogits/gogs/models"
)

func Create(req *http.Request, r render.Render) {
	if req.Method == "GET" {
		r.HTML(200, "repo/create", map[string]interface{}{
			"Title": "Create repository",
		})
		return
	}

	u := &models.User{}
	_, err := models.CreateRepository(u, "")
	r.HTML(403, "status/403", map[string]interface{}{
		"Title": fmt.Sprintf("%v", err),
	})
}

func Delete(req *http.Request, r render.Render) {
	if req.Method == "GET" {
		r.HTML(200, "repo/delete", map[string]interface{}{
			"Title": "Delete repository",
		})
		return
	}

	u := &models.User{}
	err := models.DeleteRepository(u, "")
	r.HTML(403, "status/403", map[string]interface{}{
		"Title": fmt.Sprintf("%v", err),
	})
}
