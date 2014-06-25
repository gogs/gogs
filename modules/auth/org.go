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

type CreateOrganizationForm struct {
	OrgName string `form:"orgname" binding:"Required;AlphaDashDot;MaxSize(30)"`
	Email   string `form:"email" binding:"Required;Email;MaxSize(50)"`
}

func (f *CreateOrganizationForm) Name(field string) string {
	names := map[string]string{
		"OrgName": "Organization name",
		"Email":   "E-mail address",
	}
	return names[field]
}

func (f *CreateOrganizationForm) Validate(errs *binding.Errors, req *http.Request, ctx martini.Context) {
	data := ctx.Get(reflect.TypeOf(base.TmplData{})).Interface().(base.TmplData)
	validate(errs, data, f)
}
