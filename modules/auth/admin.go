// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"github.com/Unknwon/macaron"

	"github.com/macaron-contrib/binding"
)

type AdminEditUserForm struct {
	FullName     string `form:"fullname" binding:"MaxSize(100)"`
	Email        string `binding:"Required;Email;MaxSize(50)"`
	Password     string `binding:"OmitEmpty;MinSize(6);MaxSize(255)"`
	Website      string `binding:"MaxSize(50)"`
	Location     string `binding:"MaxSize(50)"`
	Avatar       string `binding:"Required;Email;MaxSize(50)"`
	Active       bool
	Admin        bool
	AllowGitHook bool
	LoginType    int
}

func (f *AdminEditUserForm) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}
