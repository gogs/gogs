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

type CreateRepoForm struct {
	RepoName    string `form:"repo" binding:"Required;AlphaDash"`
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

func (f *CreateRepoForm) Validate(errors *base.BindingErrors, req *http.Request, context martini.Context) {
	if req.Method == "GET" || errors.Count() == 0 {
		return
	}

	data := context.Get(reflect.TypeOf(base.TmplData{})).Interface().(base.TmplData)
	data["HasError"] = true
	AssignForm(f, data)

	if len(errors.Overall) > 0 {
		for _, err := range errors.Overall {
			log.Error("CreateRepoForm.Validate: %v", err)
		}
		return
	}

	validate(errors, data, f)
}

type MigrateRepoForm struct {
	Url          string `form:"url" binding:"Url"`
	AuthUserName string `form:"auth_username"`
	AuthPasswd   string `form:"auth_password"`
	RepoName     string `form:"repo" binding:"Required;AlphaDash"`
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

func (f *MigrateRepoForm) Validate(errors *base.BindingErrors, req *http.Request, context martini.Context) {
	if req.Method == "GET" || errors.Count() == 0 {
		return
	}

	data := context.Get(reflect.TypeOf(base.TmplData{})).Interface().(base.TmplData)
	data["HasError"] = true
	AssignForm(f, data)

	if len(errors.Overall) > 0 {
		for _, err := range errors.Overall {
			log.Error("MigrateRepoForm.Validate: %v", err)
		}
		return
	}

	validate(errors, data, f)
}
