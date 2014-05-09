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

type AdminEditUserForm struct {
	Email     string `form:"email" binding:"Required;Email;MaxSize(50)"`
	Website   string `form:"website" binding:"MaxSize(50)"`
	Location  string `form:"location" binding:"MaxSize(50)"`
	Avatar    string `form:"avatar" binding:"Required;Email;MaxSize(50)"`
	Active    bool   `form:"active"`
	Admin     bool   `form:"admin"`
	LoginType int    `form:"login_type"`
}

func (f *AdminEditUserForm) Name(field string) string {
	names := map[string]string{
		"Email":    "E-mail address",
		"Website":  "Website",
		"Location": "Location",
		"Avatar":   "Gravatar Email",
	}
	return names[field]
}

func (f *AdminEditUserForm) Validate(errors *binding.Errors, req *http.Request, context martini.Context) {
	data := context.Get(reflect.TypeOf(base.TmplData{})).Interface().(base.TmplData)
	validate(errors, data, f)
}
