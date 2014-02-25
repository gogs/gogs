// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"fmt"
	"net/http"

	"github.com/martini-contrib/render"

	"github.com/gogits/gogs/models"
)

func AddPublickKey(req *http.Request, r render.Render) {
	if req.Method == "GET" {
		r.HTML(200, "user/publickey_add", map[string]interface{}{
			"Title": "Add Public Key",
		})
		return
	}

	k := &models.PublicKey{OwnerId: 1,
		Name:    req.FormValue("keyname"),
		Content: req.FormValue("key_content"),
	}
	err := models.AddPublicKey(k)
	if err != nil {
		r.HTML(403, "status/403", map[string]interface{}{
			"Title": fmt.Sprintf("%v", err),
		})
	} else {
		r.HTML(200, "user/publickey_added", map[string]interface{}{})
	}
}
