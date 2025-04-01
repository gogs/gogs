// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE.gogs file.

package form

import (
	"github.com/go-macaron/binding"
	"gopkg.in/macaron.v1"
)

type AdminCrateUser struct {
	LoginType   string `binding:"Required"`
	LoginName   string
	UserName    string `binding:"Required;AlphaDashDot;MaxSize(35)"`
	Email       string `binding:"Required;Email;MaxSize(254)"`
	PublicEmail string `binding:"Email;MaxSize(254)"`
	Password    string `binding:"MaxSize(255)"`
	SendNotify  bool
}

func (f *AdminCrateUser) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}

type AdminEditUser struct {
	LoginType        string `binding:"Required"`
	LoginName        string
	FullName         string `binding:"MaxSize(100)"`
	Email            string `binding:"Required;Email;MaxSize(254)"`
	PublicEmail      string `binding:"Required;Email;MaxSize(254)"`
	Password         string `binding:"MaxSize(255)"`
	Website          string `binding:"MaxSize(50)"`
	Location         string `binding:"MaxSize(50)"`
	MaxRepoCreation  int
	Active           bool
	Admin            bool
	AllowGitHook     bool
	AllowImportLocal bool
	ProhibitLogin    bool
}

func (f *AdminEditUser) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}
