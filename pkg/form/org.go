// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package form

import (
	"github.com/go-macaron/binding"
	"gopkg.in/macaron.v1"
)

type CreateOrg struct {
	OrgName string `binding:"Required;AlphaDashDot;MaxSize(35)" locale:"org.org_name_holder"`
}

func (f *CreateOrg) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}

type UpdateOrgSetting struct {
	Name            string `binding:"Required;AlphaDashDot;MaxSize(35)" locale:"org.org_name_holder"`
	FullName        string `binding:"MaxSize(100)"`
	Description     string `binding:"MaxSize(255)"`
	Website         string `binding:"Url;MaxSize(100)"`
	Location        string `binding:"MaxSize(50)"`
	MaxRepoCreation int
}

func (f *UpdateOrgSetting) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}

type CreateTeam struct {
	TeamName    string `binding:"Required;AlphaDashDot;MaxSize(30)"`
	Description string `binding:"MaxSize(255)"`
	Permission  string
}

func (f *CreateTeam) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}
