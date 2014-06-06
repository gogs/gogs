// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"net/http"
	"reflect"

	"github.com/go-martini/martini"

	"github.com/gogits/session"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware/binding"
)

// SignedInId returns the id of signed in user.
func SignedInId(sess session.SessionStore) int64 {
	if !models.HasEngine {
		return 0
	}

	uid := sess.Get("userId")
	if uid == nil {
		return 0
	}
	if id, ok := uid.(int64); ok {
		if _, err := models.GetUserById(id); err != nil {
			return 0
		}
		return id
	}
	return 0
}

// SignedInName returns the name of signed in user.
func SignedInName(sess session.SessionStore) string {
	uname := sess.Get("userName")
	if uname == nil {
		return ""
	}
	if s, ok := uname.(string); ok {
		return s
	}
	return ""
}

// SignedInUser returns the user object of signed user.
func SignedInUser(sess session.SessionStore) *models.User {
	uid := SignedInId(sess)
	if uid <= 0 {
		return nil
	}

	u, err := models.GetUserById(uid)
	if err != nil {
		log.Error("user.SignedInUser: %v", err)
		return nil
	}
	return u
}

// IsSignedIn check if any user has signed in.
func IsSignedIn(sess session.SessionStore) bool {
	return SignedInId(sess) > 0
}

type FeedsForm struct {
	UserId int64 `form:"userid" binding:"Required"`
	Page   int64 `form:"p"`
}

type UpdateProfileForm struct {
	UserName string `form:"username" binding:"Required;AlphaDash;MaxSize(30)"`
	FullName string `form:"fullname" binding:"MaxSize(40)"`
	Email    string `form:"email" binding:"Required;Email;MaxSize(50)"`
	Website  string `form:"website" binding:"Url;MaxSize(50)"`
	Location string `form:"location" binding:"MaxSize(50)"`
	Avatar   string `form:"avatar" binding:"Required;Email;MaxSize(50)"`
}

func (f *UpdateProfileForm) Name(field string) string {
	names := map[string]string{
		"UserName": "Username",
		"Email":    "E-mail address",
		"Website":  "Website",
		"Location": "Location",
		"Avatar":   "Gravatar Email",
	}
	return names[field]
}

func (f *UpdateProfileForm) Validate(errs *binding.Errors, req *http.Request, ctx martini.Context) {
	data := ctx.Get(reflect.TypeOf(base.TmplData{})).Interface().(base.TmplData)
	validate(errs, data, f)
}

type UpdatePasswdForm struct {
	OldPasswd    string `form:"oldpasswd" binding:"Required;MinSize(6);MaxSize(30)"`
	NewPasswd    string `form:"newpasswd" binding:"Required;MinSize(6);MaxSize(30)"`
	RetypePasswd string `form:"retypepasswd"`
}

func (f *UpdatePasswdForm) Name(field string) string {
	names := map[string]string{
		"OldPasswd":    "Old password",
		"NewPasswd":    "New password",
		"RetypePasswd": "Re-type password",
	}
	return names[field]
}

func (f *UpdatePasswdForm) Validate(errs *binding.Errors, req *http.Request, ctx martini.Context) {
	data := ctx.Get(reflect.TypeOf(base.TmplData{})).Interface().(base.TmplData)
	validate(errs, data, f)
}
