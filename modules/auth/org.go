// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"net/http"
	"reflect"

	"github.com/go-martini/martini"

	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/middleware/binding"
)

// ________                            .__                __  .__
// \_____  \_______  _________    ____ |__|____________ _/  |_|__| ____   ____
//  /   |   \_  __ \/ ___\__  \  /    \|  \___   /\__  \\   __\  |/  _ \ /    \
// /    |    \  | \/ /_/  > __ \|   |  \  |/    /  / __ \|  | |  (  <_> )   |  \
// \_______  /__|  \___  (____  /___|  /__/_____ \(____  /__| |__|\____/|___|  /
//         \/     /_____/     \/     \/         \/     \/                    \/

type CreateOrgForm struct {
	OrgName string `form:"orgname" binding:"Required;AlphaDashDot;MaxSize(30)"`
	Email   string `form:"email" binding:"Required;Email;MaxSize(50)"`
}

func (f *CreateOrgForm) Name(field string) string {
	names := map[string]string{
		"OrgName": "Organization name",
		"Email":   "E-mail address",
	}
	return names[field]
}

func (f *CreateOrgForm) Validate(errs *binding.Errors, req *http.Request, ctx martini.Context) {
	data := ctx.Get(reflect.TypeOf(base.TmplData{})).Interface().(base.TmplData)
	validate(errs, data, f)
}

type OrgSettingForm struct {
	DisplayName string `form:"display_name" binding:"Required;MaxSize(100)"`
	Email       string `form:"email" binding:"Required;Email;MaxSize(50)"`
	Description string `form:"desc" binding:"MaxSize(255)"`
	Website     string `form:"site" binding:"Url;MaxSize(100)"`
	Location    string `form:"location" binding:"MaxSize(50)"`
}

func (f *OrgSettingForm) Name(field string) string {
	names := map[string]string{
		"DisplayName": "Display name",
		"Email":       "E-mail address",
		"Description": "Description",
		"Website":     "Website address",
		"Location":    "Location",
	}
	return names[field]
}

func (f *OrgSettingForm) Validate(errors *binding.Errors, req *http.Request, context martini.Context) {
	data := context.Get(reflect.TypeOf(base.TmplData{})).Interface().(base.TmplData)
	validate(errors, data, f)
}

// ___________
// \__    ___/___ _____    _____
//   |    |_/ __ \\__  \  /     \
//   |    |\  ___/ / __ \|  Y Y  \
//   |____| \___  >____  /__|_|  /
//              \/     \/      \/

type CreateTeamForm struct {
	TeamName    string `form:"name" binding:"Required;AlphaDashDot;MaxSize(30)"`
	Description string `form:"desc" binding:"MaxSize(255)"`
	Permission  string `form:"permission"`
}

func (f *CreateTeamForm) Name(field string) string {
	names := map[string]string{
		"TeamName":    "Team name",
		"Description": "Team description",
	}
	return names[field]
}

func (f *CreateTeamForm) Validate(errs *binding.Errors, req *http.Request, ctx martini.Context) {
	data := ctx.Get(reflect.TypeOf(base.TmplData{})).Interface().(base.TmplData)
	validate(errs, data, f)
}
