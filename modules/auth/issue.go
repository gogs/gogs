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

type CreateIssueForm struct {
	IssueName   string `form:"title" binding:"Required;MaxSize(50)"`
	MilestoneId int64  `form:"milestoneid"`
	AssigneeId  int64  `form:"assigneeid"`
	Labels      string `form:"labels"`
	Content     string `form:"content"`
}

func (f *CreateIssueForm) Name(field string) string {
	names := map[string]string{
		"IssueName": "Issue name",
	}
	return names[field]
}

func (f *CreateIssueForm) Validate(errors *binding.Errors, req *http.Request, context martini.Context) {
	data := context.Get(reflect.TypeOf(base.TmplData{})).Interface().(base.TmplData)
	validate(errors, data, f)
}
