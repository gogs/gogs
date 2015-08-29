// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"github.com/Unknwon/macaron"
	"github.com/macaron-contrib/binding"
)

// _______________________________________    _________.______________________ _______________.___.
// \______   \_   _____/\______   \_____  \  /   _____/|   \__    ___/\_____  \\______   \__  |   |
//  |       _/|    __)_  |     ___//   |   \ \_____  \ |   | |    |    /   |   \|       _//   |   |
//  |    |   \|        \ |    |   /    |    \/        \|   | |    |   /    |    \    |   \\____   |
//  |____|_  /_______  / |____|   \_______  /_______  /|___| |____|   \_______  /____|_  // ______|
//         \/        \/                   \/        \/                        \/       \/ \/

type CreateRepoForm struct {
	Uid         int64  `binding:"Required"`
	RepoName    string `binding:"Required;AlphaDashDot;MaxSize(100)"`
	Private     bool
	Description string `binding:"MaxSize(255)"`
	AutoInit    bool
	Gitignores  string
	License     string
	Readme      string
}

func (f *CreateRepoForm) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}

type MigrateRepoForm struct {
	CloneAddr    string `binding:"Required"`
	AuthUsername string
	AuthPassword string
	Mirror       bool
	Uid          int64  `binding:"Required"`
	RepoName     string `binding:"Required;AlphaDashDot;MaxSize(100)"`
	Private      bool
	Description  string `binding:"MaxSize(255)"`
}

func (f *MigrateRepoForm) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}

type RepoSettingForm struct {
	RepoName    string `form:"repo_name" binding:"Required;AlphaDashDot;MaxSize(100)"`
	Description string `form:"desc" binding:"MaxSize(255)"`
	Website     string `form:"site" binding:"Url;MaxSize(100)"`
	Branch      string `form:"branch"`
	Interval    int    `form:"interval"`
	Private     bool   `form:"private"`
}

func (f *RepoSettingForm) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}

//  __      __      ___.   .__    .__            __
// /  \    /  \ ____\_ |__ |  |__ |  |__   ____ |  | __
// \   \/\/   // __ \| __ \|  |  \|  |  \ /  _ \|  |/ /
//  \        /\  ___/| \_\ \   Y  \   Y  (  <_> )    <
//   \__/\  /  \___  >___  /___|  /___|  /\____/|__|_ \
//        \/       \/    \/     \/     \/            \/

type WebhookForm struct {
	Events string
	Create bool
	Push   bool
	Active bool
}

func (f WebhookForm) PushOnly() bool {
	return f.Events == "push_only"
}

func (f WebhookForm) SendEverything() bool {
	return f.Events == "send_everything"
}

func (f WebhookForm) ChooseEvents() bool {
	return f.Events == "choose_events"
}

type NewWebhookForm struct {
	PayloadURL  string `binding:"Required;Url"`
	ContentType int    `binding:"Required"`
	Secret      string
	WebhookForm
}

func (f *NewWebhookForm) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}

type NewSlackHookForm struct {
	PayloadURL string `binding:"Required`
	Channel    string `binding:"Required"`
	Username   string
	IconURL    string
	Color      string
	WebhookForm
}

func (f *NewSlackHookForm) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}

// .___
// |   | ______ ________ __   ____
// |   |/  ___//  ___/  |  \_/ __ \
// |   |\___ \ \___ \|  |  /\  ___/
// |___/____  >____  >____/  \___  >
//          \/     \/            \/

type CreateIssueForm struct {
	Title       string `binding:"Required;MaxSize(255)"`
	LabelIDs    string `form:"label_ids"`
	MilestoneID int64
	AssigneeID  int64
	Content     string
	Attachments []string
}

func (f *CreateIssueForm) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}

type CreateCommentForm struct {
	Content     string
	Status      string `binding:"OmitEmpty;In(reopen,close)"`
	Attachments []string
}

func (f *CreateCommentForm) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}

//    _____  .__.__                   __
//   /     \ |__|  |   ____   _______/  |_  ____   ____   ____
//  /  \ /  \|  |  | _/ __ \ /  ___/\   __\/  _ \ /    \_/ __ \
// /    Y    \  |  |_\  ___/ \___ \  |  | (  <_> )   |  \  ___/
// \____|__  /__|____/\___  >____  > |__|  \____/|___|  /\___  >
//         \/             \/     \/                   \/     \/

type CreateMilestoneForm struct {
	Title    string `binding:"Required;MaxSize(50)"`
	Content  string
	Deadline string
}

func (f *CreateMilestoneForm) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}

// .____          ___.          .__
// |    |   _____ \_ |__   ____ |  |
// |    |   \__  \ | __ \_/ __ \|  |
// |    |___ / __ \| \_\ \  ___/|  |__
// |_______ (____  /___  /\___  >____/
//         \/    \/    \/     \/

type CreateLabelForm struct {
	ID    int64
	Title string `binding:"Required;MaxSize(50)" locale:"repo.issues.label_name"`
	Color string `binding:"Required;Size(7)" locale:"repo.issues.label_color"`
}

func (f *CreateLabelForm) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
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

func (f *NewReleaseForm) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}

type EditReleaseForm struct {
	Title      string `form:"title" binding:"Required"`
	Content    string `form:"content" binding:"Required"`
	Draft      string `form:"draft"`
	Prerelease bool   `form:"prerelease"`
}

func (f *EditReleaseForm) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}
