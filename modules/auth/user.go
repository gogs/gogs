// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"github.com/codegangsta/martini"
	"github.com/martini-contrib/render"
	"github.com/martini-contrib/sessions"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
)

func SignedInId(session sessions.Session) int64 {
	userId := session.Get("userId")
	if userId == nil {
		return 0
	}
	if s, ok := userId.(int64); ok {
		if _, err := models.GetUserById(s); err != nil {
			return 0
		}
		return s
	}
	return 0
}

func SignedInName(session sessions.Session) string {
	userName := session.Get("userName")
	if userName == nil {
		return ""
	}
	if s, ok := userName.(string); ok {
		return s
	}
	return ""
}

func SignedInUser(session sessions.Session) *models.User {
	id := SignedInId(session)
	if id <= 0 {
		return nil
	}

	user, err := models.GetUserById(id)
	if err != nil {
		log.Error("user.SignedInUser: %v", err)
		return nil
	}
	return user
}

func IsSignedIn(session sessions.Session) bool {
	return SignedInId(session) > 0
}

// SignInRequire checks user status from session.
// It will assign correspoding values to
// template data map if user has signed in.
func SignInRequire(redirect bool) martini.Handler {
	return func(r render.Render, data base.TmplData, session sessions.Session) {
		if !IsSignedIn(session) {
			if redirect {
				r.Redirect("/")
			}
			return
		}

		data["IsSigned"] = true
		data["SignedUserId"] = SignedInId(session)
		data["SignedUserName"] = SignedInName(session)
		data["SignedAvatar"] = SignedInUser(session).Avatar
	}
}

func SignOutRequire() martini.Handler {
	return func(r render.Render, session sessions.Session) {
		if IsSignedIn(session) {
			r.Redirect("/")
		}
	}
}
