// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"fmt"
	"net/http"

	"github.com/martini-contrib/render"
	"github.com/martini-contrib/sessions"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
)

func AddPublicKey(req *http.Request, r render.Render, session sessions.Session) {
	if req.Method == "GET" {
		r.HTML(200, "user/publickey_add", map[string]interface{}{
			"Title":    "Add Public Key",
			"IsSigned": auth.IsSignedIn(session),
		})
		return
	}

	k := &models.PublicKey{OwnerId: auth.SignedInId(session),
		Name:    req.FormValue("keyname"),
		Content: req.FormValue("key_content"),
	}
	err := models.AddPublicKey(k)
	if err != nil {
		r.HTML(403, "status/403", map[string]interface{}{
			"Title":    fmt.Sprintf("%v", err),
			"IsSigned": auth.IsSignedIn(session),
		})
	} else {
		r.HTML(200, "user/publickey_added", map[string]interface{}{})
	}
}

func ListPublicKey(req *http.Request, r render.Render, session sessions.Session) {
	keys, err := models.ListPublicKey(auth.SignedInId(session))
	if err != nil {
		r.HTML(200, "base/error", map[string]interface{}{
			"Error":    fmt.Sprintf("%v", err),
			"IsSigned": auth.IsSignedIn(session),
		})
		return
	}

	r.HTML(200, "user/publickey_list", map[string]interface{}{
		"Title":    "repositories",
		"Keys":     keys,
		"IsSigned": auth.IsSignedIn(session),
	})
}
