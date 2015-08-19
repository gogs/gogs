// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"mime/multipart"

	"github.com/Unknwon/macaron"
	"github.com/macaron-contrib/binding"
)

type InstallForm struct {
	DbType   string `binding:"Required"`
	DbHost   string
	DbUser   string
	DbPasswd string
	DbName   string
	SSLMode  string
	DbPath   string

	AppName      string `binding:"Required" locale:"install.app_name"`
	RepoRootPath string `binding:"Required"`
	RunUser      string `binding:"Required"`
	Domain       string `binding:"Required"`
	SSHPort      int
	HTTPPort     string `binding:"Required"`
	AppUrl       string `binding:"Required"`

	SMTPHost        string
	SMTPFrom        string
	SMTPEmail       string `binding:"OmitEmpty;Email;MaxSize(50)" locale:"install.mailer_user"`
	SMTPPasswd      string
	RegisterConfirm bool
	MailNotify      bool

	OfflineMode         bool
	DisableRegistration bool
	RequireSignInView   bool

	AdminName          string `binding:"OmitEmpty;AlphaDashDot;MaxSize(30)" locale:"install.admin_name"`
	AdminPasswd        string `binding:"OmitEmpty;MaxSize(255)" locale:"install.admin_password"`
	AdminConfirmPasswd string
	AdminEmail         string `binding:"OmitEmpty;Email;MaxSize(50)" locale:"install.admin_email"`
}

func (f *InstallForm) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}

//    _____   ____ _________________ ___
//   /  _  \ |    |   \__    ___/   |   \
//  /  /_\  \|    |   / |    | /    ~    \
// /    |    \    |  /  |    | \    Y    /
// \____|__  /______/   |____|  \___|_  /
//         \/                         \/

type RegisterForm struct {
	UserName  string `form:"uname" binding:"Required;AlphaDashDot;MaxSize(35)"`
	Email     string `form:"email" binding:"Required;Email;MaxSize(50)"`
	Password  string `form:"password" binding:"Required;MaxSize(255)"`
	Retype    string `form:"retype"`
	LoginType string `form:"logintype"`
	LoginName string `form:"loginname"`
}

func (f *RegisterForm) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}

type SignInForm struct {
	UserName string `form:"uname" binding:"Required;MaxSize(35)"`
	Password string `form:"password" binding:"Required;MaxSize(255)"`
	Remember bool   `form:"remember"`
}

func (f *SignInForm) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}

//   __________________________________________.___ _______    ________  _________
//  /   _____/\_   _____/\__    ___/\__    ___/|   |\      \  /  _____/ /   _____/
//  \_____  \  |    __)_   |    |     |    |   |   |/   |   \/   \  ___ \_____  \
//  /        \ |        \  |    |     |    |   |   /    |    \    \_\  \/        \
// /_______  //_______  /  |____|     |____|   |___\____|__  /\______  /_______  /
//         \/         \/                                   \/        \/        \/

type UpdateProfileForm struct {
	UserName string `form:"uname" binding:"Required;MaxSize(35)"`
	FullName string `form:"fullname" binding:"MaxSize(100)"`
	Email    string `form:"email" binding:"Required;Email;MaxSize(50)"`
	Website  string `form:"website" binding:"Url;MaxSize(100)"`
	Location string `form:"location" binding:"MaxSize(50)"`
	Avatar   string `form:"avatar" binding:"Required;Email;MaxSize(50)"`
}

func (f *UpdateProfileForm) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}

type UploadAvatarForm struct {
	Enable bool                  `form:"enable"`
	Avatar *multipart.FileHeader `form:"avatar"`
}

func (f *UploadAvatarForm) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}

type AddEmailForm struct {
	Email string `binding:"Required;Email;MaxSize(50)"`
}

func (f *AddEmailForm) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}

type ChangePasswordForm struct {
	OldPassword string `form:"old_password" binding:"Required;MinSize(1);MaxSize(255)"`
	Password    string `form:"password" binding:"Required;MaxSize(255)"`
	Retype      string `form:"retype"`
}

func (f *ChangePasswordForm) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}

type AddSSHKeyForm struct {
	Title   string `binding:"Required;MaxSize(50)"`
	Content string `binding:"Required"`
}

func (f *AddSSHKeyForm) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}

type NewAccessTokenForm struct {
	Name string `binding:"Required"`
}

func (f *NewAccessTokenForm) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}
