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

type AddSSHKeyForm struct {
	KeyName    string `form:"keyname" binding:"Required"`
	KeyContent string `form:"key_content" binding:"Required"`
}

func (f *AddSSHKeyForm) Name(field string) string {
	names := map[string]string{
		"KeyName":    "SSH key name",
		"KeyContent": "SSH key content",
	}
	return names[field]
}

func (f *AddSSHKeyForm) Validate(errors *binding.Errors, req *http.Request, context martini.Context) {
	data := context.Get(reflect.TypeOf(base.TmplData{})).Interface().(base.TmplData)
	validate(errors, data, f)
}
