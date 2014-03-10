// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/martini-contrib/render"
	"github.com/martini-contrib/sessions"
	"net/http"
	"strconv"
)

func Setting(r render.Render, data base.TmplData, session sessions.Session) {
	data["Title"] = "Setting"
	data["PageIsUserSetting"] = true
	r.HTML(200, "user/setting", data)
}

func SettingSSHKeys(r render.Render, data base.TmplData, req *http.Request, session sessions.Session) {
	// del ssh ky
	if req.Method == "DELETE" || req.FormValue("_method") == "DELETE" {
		id, err := strconv.ParseInt(req.FormValue("id"), 10, 64)
		if err != nil {
			data["ErrorMsg"] = err
			log.Error("ssh.DelPublicKey: %v", err)
			r.JSON(200, map[string]interface{}{
				"ok":  false,
				"err": err.Error(),
			})
			return
		}
		k := &models.PublicKey{
			Id:      id,
			OwnerId: auth.SignedInId(session),
		}
		err = models.DeletePublicKey(k)
		if err != nil {
			data["ErrorMsg"] = err
			log.Error("ssh.DelPublicKey: %v", err)
			r.JSON(200, map[string]interface{}{
				"ok":  false,
				"err": err.Error(),
			})
		} else {
			r.JSON(200, map[string]interface{}{
				"ok": true,
			})
		}
		return
	}
	// add ssh key
	if req.Method == "POST" {
		k := &models.PublicKey{OwnerId: auth.SignedInId(session),
			Name:    req.FormValue("keyname"),
			Content: req.FormValue("key_content"),
		}
		err := models.AddPublicKey(k)
		if err != nil {
			data["ErrorMsg"] = err
			log.Error("ssh.AddPublicKey: %v", err)
			r.HTML(200, "base/error", data)
			return
		} else {
			data["AddSSHKeySuccess"] = true
		}
	}
	// get keys
	keys, err := models.ListPublicKey(auth.SignedInId(session))
	if err != nil {
		data["ErrorMsg"] = err
		log.Error("ssh.ListPublicKey: %v", err)
		r.HTML(200, "base/error", data)
		return
	}

	// set to template
	data["Title"] = "SSH Keys"
	data["PageIsUserSetting"] = true
	data["Keys"] = keys
	r.HTML(200, "user/publickey", data)
}
