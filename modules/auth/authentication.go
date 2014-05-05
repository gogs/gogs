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
	"github.com/gogits/gogs/modules/middleware/binding"
)

type AuthenticationForm struct {
	Id         int64  `form:"id"`
	Type       int    `form:"type"`
	AuthName   string `form:"name" binding:"Required;MaxSize(50)"`
	Domain     string `form:"domain" binding:"Required"`
	Host       string `form:"host" binding:"Required"`
	Port       int    `form:"port" binding:"Required"`
	BaseDN     string `form:"base_dn" binding:"Required"`
	Attributes string `form:"attributes" binding:"Required"`
	Filter     string `form:"filter" binding:"Required"`
	MsAdSA     string `form:"ms_ad_sa" binding:"Required"`
	IsActived  bool   `form:"is_actived"`
}

func (f *AuthenticationForm) Name(field string) string {
	names := map[string]string{
		"AuthName":   "Authentication's name",
		"Domain":     "Domain name",
		"Host":       "Host address",
		"Port":       "Port Number",
		"BaseDN":     "Base DN",
		"Attributes": "Search attributes",
		"Filter":     "Search filter",
		"MsAdSA":     "Ms Ad SA",
	}
	return names[field]
}

func (f *AuthenticationForm) Validate(errors *binding.BindingErrors, req *http.Request, context martini.Context) {
	if req.Method == "GET" || errors.Count() == 0 {
		return
	}

	data := context.Get(reflect.TypeOf(base.TmplData{})).Interface().(base.TmplData)
	data["HasError"] = true
	AssignForm(f, data)

	if len(errors.Overall) > 0 {
		for _, err := range errors.Overall {
			log.Error("AuthenticationForm.Validate: %v", err)
		}
		return
	}

	validate(errors, data, f)
}
