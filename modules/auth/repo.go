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

type CreateRepoForm struct {
	RepoName    string `form:"repo" binding:"Required;AlphaDash;MaxSize(100)"`
	Private     bool   `form:"private"`
	Description string `form:"desc" binding:"MaxSize(100)"`
	Language    string `form:"language"`
	License     string `form:"license"`
	InitReadme  bool   `form:"initReadme"`
}

func (f *CreateRepoForm) Name(field string) string {
	names := map[string]string{
		"RepoName":    "Repository name",
		"Description": "Description",
	}
	return names[field]
}

func (f *CreateRepoForm) Validate(errors *binding.Errors, req *http.Request, context martini.Context) {
	data := context.Get(reflect.TypeOf(base.TmplData{})).Interface().(base.TmplData)
	validate(errors, data, f)
}

type MigrateRepoForm struct {
	Url          string `form:"url" binding:"Url"`
	AuthUserName string `form:"auth_username"`
	AuthPasswd   string `form:"auth_password"`
	RepoName     string `form:"repo" binding:"Required;AlphaDash;MaxSize(100)"`
	Mirror       bool   `form:"mirror"`
	Private      bool   `form:"private"`
	Description  string `form:"desc" binding:"MaxSize(100)"`
}

func (f *MigrateRepoForm) Name(field string) string {
	names := map[string]string{
		"Url":         "Migration URL",
		"RepoName":    "Repository name",
		"Description": "Description",
	}
	return names[field]
}

func (f *MigrateRepoForm) Validate(errors *binding.Errors, req *http.Request, context martini.Context) {
	data := context.Get(reflect.TypeOf(base.TmplData{})).Interface().(base.TmplData)
	validate(errors, data, f)
}

type RepoSettingForm struct {
	RepoName    string `form:"name" binding:"Required;AlphaDash;MaxSize(100)"`
	Description string `form:"desc" binding:"MaxSize(100)"`
	Website     string `form:"url" binding:"Url;MaxSize(100)"`
	Branch      string `form:"branch"`
	Interval    int    `form:"interval"`
	Private     bool   `form:"private"`
	GoGet       bool   `form:"goget"`
}

func (f *RepoSettingForm) Name(field string) string {
	names := map[string]string{
		"RepoName":    "Repository name",
		"Description": "Description",
		"Website":     "Website address",
	}
	return names[field]
}

func (f *RepoSettingForm) Validate(errors *binding.Errors, req *http.Request, context martini.Context) {
	data := context.Get(reflect.TypeOf(base.TmplData{})).Interface().(base.TmplData)
	validate(errors, data, f)
}

type NewWebhookForm struct {
	Url         string `form:"url" binding:"Required;Url"`
	ContentType string `form:"content_type" binding:"Required"`
	Secret      string `form:"secret""`
	PushOnly    bool   `form:"push_only"`
	Active      bool   `form:"active"`
}

func (f *NewWebhookForm) Name(field string) string {
	names := map[string]string{
		"Url":         "Payload URL",
		"ContentType": "Content type",
	}
	return names[field]
}

func (f *NewWebhookForm) Validate(errors *binding.Errors, req *http.Request, context martini.Context) {
	data := context.Get(reflect.TypeOf(base.TmplData{})).Interface().(base.TmplData)
	validate(errors, data, f)
}
