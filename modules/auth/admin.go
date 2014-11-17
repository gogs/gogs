// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"github.com/Unknwon/macaron"

	"github.com/macaron-contrib/binding"
)

type AdminEditUserForm struct {
	Email        string `form:"email" binding:"Required;Email;MaxSize(50)"`
	Passwd       string `form:"password"`
	Website      string `form:"website" binding:"MaxSize(50)"`
	Location     string `form:"location" binding:"MaxSize(50)"`
	Avatar       string `form:"avatar" binding:"Required;Email;MaxSize(50)"`
	Active       bool   `form:"active"`
	Admin        bool   `form:"admin"`
	AllowGitHook bool   `form:"allow_git_hook"`
	LoginType    int    `form:"login_type"`
}

func (f *AdminEditUserForm) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}
