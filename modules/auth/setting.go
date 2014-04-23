// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"net/http"
	"reflect"
	"strings"

	"github.com/go-martini/martini"

	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
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

func (f *AddSSHKeyForm) Validate(errors *base.BindingErrors, req *http.Request, context martini.Context) {
	data := context.Get(reflect.TypeOf(base.TmplData{})).Interface().(base.TmplData)
	AssignForm(f, data)

	if req.Method == "GET" || errors.Count() == 0 {
		if req.Method == "POST" &&
			(len(f.KeyContent) < 100 || !strings.HasPrefix(f.KeyContent, "ssh-rsa")) {
			data["HasError"] = true
			data["ErrorMsg"] = "SSH key content is not valid"
		}
		return
	}

	data["HasError"] = true
	if len(errors.Overall) > 0 {
		for _, err := range errors.Overall {
			log.Error("AddSSHKeyForm.Validate: %v", err)
		}
		return
	}

	validate(errors, data, f)
}
