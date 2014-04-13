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
)

// SignedInId returns the id of signed in user.
func SignedInId(session session.SessionStore) int64 {
	if !models.HasEngine {
		return 0
	}

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

// SignedInName returns the name of signed in user.
func SignedInName(session session.SessionStore) string {
	userName := session.Get("userName")
	if userName == nil {
		return ""
	}
	if s, ok := userName.(string); ok {
		return s
	}
	return ""
}

// SignedInUser returns the user object of signed user.
func SignedInUser(session session.SessionStore) *models.User {
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

// IsSignedIn check if any user has signed in.
func IsSignedIn(session session.SessionStore) bool {
	return SignedInId(session) > 0
}

type FeedsForm struct {
	UserId int64 `form:"userid" binding:"Required"`
	Page   int64 `form:"p"`
}

type UpdateProfileForm struct {
	UserName string `form:"username" binding:"Required;AlphaDash;MaxSize(30)"`
	Email    string `form:"email" binding:"Required;Email;MaxSize(50)"`
	Website  string `form:"website" binding:"MaxSize(50)"`
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

func (f *UpdateProfileForm) Validate(errors *base.BindingErrors, req *http.Request, context martini.Context) {
	if req.Method == "GET" || errors.Count() == 0 {
		return
	}

	data := context.Get(reflect.TypeOf(base.TmplData{})).Interface().(base.TmplData)
	data["HasError"] = true

	if len(errors.Overall) > 0 {
		for _, err := range errors.Overall {
			log.Error("UpdateProfileForm.Validate: %v", err)
		}
		return
	}

	validate(errors, data, f)
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

func (f *UpdatePasswdForm) Validate(errors *base.BindingErrors, req *http.Request, context martini.Context) {
	if req.Method == "GET" || errors.Count() == 0 {
		return
	}

	data := context.Get(reflect.TypeOf(base.TmplData{})).Interface().(base.TmplData)
	data["HasError"] = true

	if len(errors.Overall) > 0 {
		for _, err := range errors.Overall {
			log.Error("UpdatePasswdForm.Validate: %v", err)
		}
		return
	}

	validate(errors, data, f)
}
