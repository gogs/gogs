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

type NewReleaseForm struct {
	TagName    string `form:"tag_name" binding:"Required"`
	Title      string `form:"title" binding:"Required"`
	Content    string `form:"content" binding:"Required"`
	Prerelease bool   `form:"prerelease"`
}

func (f *NewReleaseForm) Name(field string) string {
	names := map[string]string{
		"TagName": "Tag name",
		"Title":   "Release title",
		"Content": "Release content",
	}
	return names[field]
}

func (f *NewReleaseForm) Validate(errors *base.BindingErrors, req *http.Request, context martini.Context) {
	if req.Method == "GET" || errors.Count() == 0 {
		return
	}

	data := context.Get(reflect.TypeOf(base.TmplData{})).Interface().(base.TmplData)
	data["HasError"] = true
	AssignForm(f, data)

	if len(errors.Overall) > 0 {
		for _, err := range errors.Overall {
			log.Error("NewReleaseForm.Validate: %v", err)
		}
		return
	}

	validate(errors, data, f)
}
