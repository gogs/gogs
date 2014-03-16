// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"net/http"
	"reflect"

	"github.com/codegangsta/martini"

	"github.com/gogits/binding"

	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
)

type CreateRepoForm struct {
	UserId      int64  `form:"userId"`
	RepoName    string `form:"repo" binding:"Required;AlphaDash"`
	Visibility  string `form:"visibility"`
	Description string `form:"desc" binding:"MaxSize(100)"`
	Language    string `form:"language"`
	License     string `form:"license"`
	InitReadme  string `form:"initReadme"`
}

func (f *CreateRepoForm) Name(field string) string {
	names := map[string]string{
		"RepoName":    "Repository name",
		"Description": "Description",
	}
	return names[field]
}

func (f *CreateRepoForm) Validate(errors *binding.Errors, req *http.Request, context martini.Context) {
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

type DeleteRepoForm struct {
	UserId   int64  `form:"userId" binding:"Required"`
	UserName string `form:"userName" binding:"Required"`
	RepoId   int64  `form:"repoId" binding:"Required"`
}
