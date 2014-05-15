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

type AuthenticationForm struct {
	Id                int64  `form:"id"`
	Type              int    `form:"type"`
	AuthName          string `form:"name" binding:"Required;MaxSize(50)"`
	Domain            string `form:"domain"`
	Host              string `form:"host"`
	Port              int    `form:"port"`
	UseSSL            bool   `form:"usessl"`
	BaseDN            string `form:"base_dn"`
	Attributes        string `form:"attributes"`
	Filter            string `form:"filter"`
	MsAdSA            string `form:"ms_ad_sa"`
	IsActived         bool   `form:"is_actived"`
	SmtpAuth          string `form:"smtpauth"`
	SmtpHost          string `form:"smtphost"`
	SmtpPort          int    `form:"smtpport"`
	Tls               bool   `form:"tls"`
	AllowAutoRegister bool   `form:"allowautoregister"`
}

func (f *AuthenticationForm) Name(field string) string {
	names := map[string]string{
		"AuthName":   "Authentication's name",
		"Domain":     "Domain name",
		"Host":       "Host address",
		"Port":       "Port Number",
		"UseSSL":     "Use SSL",
		"BaseDN":     "Base DN",
		"Attributes": "Search attributes",
		"Filter":     "Search filter",
		"MsAdSA":     "Ms Ad SA",
	}
	return names[field]
}

func (f *AuthenticationForm) Validate(errors *binding.Errors, req *http.Request, context martini.Context) {
	data := context.Get(reflect.TypeOf(base.TmplData{})).Interface().(base.TmplData)
	validate(errors, data, f)
}
