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

// __________                           .__  __
// \______   \ ____ ______   ____  _____|__|/  |_  ___________ ___.__.
//  |       _// __ \\____ \ /  _ \/  ___/  \   __\/  _ \_  __ <   |  |
//  |    |   \  ___/|  |_> >  <_> )___ \|  ||  | (  <_> )  | \/\___  |
//  |____|_  /\___  >   __/ \____/____  >__||__|  \____/|__|   / ____|
//         \/     \/|__|              \/                       \/

type CreateRepoForm struct {
	Uid         int64  `form:"uid" binding:"Required"`
	RepoName    string `form:"repo" binding:"Required;AlphaDash;MaxSize(100)"`
	Private     bool   `form:"private"`
	Description string `form:"desc" binding:"MaxSize(255)"`
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
	Uid          int64  `form:"uid" binding:"Required"`
	RepoName     string `form:"repo" binding:"Required;AlphaDash;MaxSize(100)"`
	Mirror       bool   `form:"mirror"`
	Private      bool   `form:"private"`
	Description  string `form:"desc" binding:"MaxSize(255)"`
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
	Description string `form:"desc" binding:"MaxSize(255)"`
	Website     string `form:"site" binding:"Url;MaxSize(100)"`
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

//  __      __      ___.   .__    .__            __
// /  \    /  \ ____\_ |__ |  |__ |  |__   ____ |  | __
// \   \/\/   // __ \| __ \|  |  \|  |  \ /  _ \|  |/ /
//  \        /\  ___/| \_\ \   Y  \   Y  (  <_> )    <
//   \__/\  /  \___  >___  /___|  /___|  /\____/|__|_ \
//        \/       \/    \/     \/     \/            \/

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

// .___
// |   | ______ ________ __   ____
// |   |/  ___//  ___/  |  \_/ __ \
// |   |\___ \ \___ \|  |  /\  ___/
// |___/____  >____  >____/  \___  >
//          \/     \/            \/

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

//    _____  .__.__                   __
//   /     \ |__|  |   ____   _______/  |_  ____   ____   ____
//  /  \ /  \|  |  | _/ __ \ /  ___/\   __\/  _ \ /    \_/ __ \
// /    Y    \  |  |_\  ___/ \___ \  |  | (  <_> )   |  \  ___/
// \____|__  /__|____/\___  >____  > |__|  \____/|___|  /\___  >
//         \/             \/     \/                   \/     \/

type CreateMilestoneForm struct {
	Title    string `form:"title" binding:"Required;MaxSize(50)"`
	Content  string `form:"content"`
	Deadline string `form:"due_date"`
}

func (f *CreateMilestoneForm) Name(field string) string {
	names := map[string]string{
		"Title": "Milestone name",
	}
	return names[field]
}

func (f *CreateMilestoneForm) Validate(errors *binding.Errors, req *http.Request, context martini.Context) {
	data := context.Get(reflect.TypeOf(base.TmplData{})).Interface().(base.TmplData)
	validate(errors, data, f)
}

// .____          ___.          .__
// |    |   _____ \_ |__   ____ |  |
// |    |   \__  \ | __ \_/ __ \|  |
// |    |___ / __ \| \_\ \  ___/|  |__
// |_______ (____  /___  /\___  >____/
//         \/    \/    \/     \/

type CreateLabelForm struct {
	Title string `form:"title" binding:"Required;MaxSize(50)"`
	Color string `form:"color" binding:"Required;Size(7)"`
}

func (f *CreateLabelForm) Name(field string) string {
	names := map[string]string{
		"Title": "Label name",
		"Color": "Label color",
	}
	return names[field]
}

func (f *CreateLabelForm) Validate(errors *binding.Errors, req *http.Request, context martini.Context) {
	data := context.Get(reflect.TypeOf(base.TmplData{})).Interface().(base.TmplData)
	validate(errors, data, f)
}

// __________       .__
// \______   \ ____ |  |   ____ _____    ______ ____
//  |       _// __ \|  | _/ __ \\__  \  /  ___// __ \
//  |    |   \  ___/|  |_\  ___/ / __ \_\___ \\  ___/
//  |____|_  /\___  >____/\___  >____  /____  >\___  >
//         \/     \/          \/     \/     \/     \/

type NewReleaseForm struct {
	TagName    string `form:"tag_name" binding:"Required"`
	Target     string `form:"tag_target" binding:"Required"`
	Title      string `form:"title" binding:"Required"`
	Content    string `form:"content" binding:"Required"`
	Draft      string `form:"draft"`
	Prerelease bool   `form:"prerelease"`
}

func (f *NewReleaseForm) Name(field string) string {
	names := map[string]string{
		"TagName": "Tag name",
		"Target":  "Target",
		"Title":   "Release title",
		"Content": "Release content",
	}
	return names[field]
}

func (f *NewReleaseForm) Validate(errors *binding.Errors, req *http.Request, context martini.Context) {
	data := context.Get(reflect.TypeOf(base.TmplData{})).Interface().(base.TmplData)
	validate(errors, data, f)
}

type EditReleaseForm struct {
	Target     string `form:"tag_target" binding:"Required"`
	Title      string `form:"title" binding:"Required"`
	Content    string `form:"content" binding:"Required"`
	Draft      string `form:"draft"`
	Prerelease bool   `form:"prerelease"`
}

func (f *EditReleaseForm) Name(field string) string {
	names := map[string]string{
		"Target":  "Target",
		"Title":   "Release title",
		"Content": "Release content",
	}
	return names[field]
}

func (f *EditReleaseForm) Validate(errors *binding.Errors, req *http.Request, context martini.Context) {
	data := context.Get(reflect.TypeOf(base.TmplData{})).Interface().(base.TmplData)
	validate(errors, data, f)
}
