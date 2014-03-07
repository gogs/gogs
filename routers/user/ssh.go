// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"net/http"

	"github.com/martini-contrib/render"
	"github.com/martini-contrib/sessions"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
)

func AddPublicKey(req *http.Request, data base.TmplData, r render.Render, session sessions.Session) {
	data["Title"] = "Add Public Key"

	if req.Method == "GET" {
		r.HTML(200, "user/publickey_add", data)
		return
	}

	k := &models.PublicKey{OwnerId: auth.SignedInId(session),
		Name:    req.FormValue("keyname"),
		Content: req.FormValue("key_content"),
	}
	err := models.AddPublicKey(k)
	if err != nil {
		data["ErrorMsg"] = err
		log.Error("ssh.AddPublicKey: %v", err)
		r.HTML(200, "base/error", data)
	} else {
		r.HTML(200, "user/publickey_added", data)
	}
}

func ListPublicKey(req *http.Request, data base.TmplData, r render.Render, session sessions.Session) {
	data["Title"] = "Public Keys"

	keys, err := models.ListPublicKey(auth.SignedInId(session))
	if err != nil {
		data["ErrorMsg"] = err
		log.Error("ssh.ListPublicKey: %v", err)
		r.HTML(200, "base/error", data)
		return
	}

	data["Keys"] = keys
	r.HTML(200, "user/publickey_list", data)
}
