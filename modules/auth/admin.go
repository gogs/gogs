// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"net/http"
	"reflect"

	"github.com/go-martini/martini"

	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
)

type AdminEditUserForm struct {
	Email    string `form:"email" binding:"Required;Email;MaxSize(50)"`
	Website  string `form:"website" binding:"MaxSize(50)"`
	Location string `form:"location" binding:"MaxSize(50)"`
	Avatar   string `form:"avatar" binding:"Required;Email;MaxSize(50)"`
	Active   string `form:"active"`
	Admin    string `form:"admin"`
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

func (f *AdminEditUserForm) Validate(errors *base.BindingErrors, req *http.Request, context martini.Context) {
	if req.Method == "GET" || errors.Count() == 0 {
		return
	}

	data := context.Get(reflect.TypeOf(base.TmplData{})).Interface().(base.TmplData)
	data["HasError"] = true
	AssignForm(f, data)

	if len(errors.Overall) > 0 {
		for _, err := range errors.Overall {
			log.Error("AdminEditUserForm.Validate: %v", err)
		}
		return
	}

	validate(errors, data, f)
}
