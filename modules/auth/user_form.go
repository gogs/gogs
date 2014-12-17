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
	Database        string `form:"database" binding:"Required"`
	DbHost          string `form:"host"`
	DbUser          string `form:"user"`
	DbPasswd        string `form:"passwd"`
	DatabaseName    string `form:"database_name"`
	SslMode         string `form:"ssl_mode"`
	DatabasePath    string `form:"database_path"`
	RepoRootPath    string `form:"repo_path" binding:"Required"`
	RunUser         string `form:"run_user" binding:"Required"`
	Domain          string `form:"domain" binding:"Required"`
	AppUrl          string `form:"app_url" binding:"Required"`
	SmtpHost        string `form:"smtp_host"`
	SmtpEmail       string `form:"mailer_user"`
	SmtpPasswd      string `form:"mailer_pwd"`
	RegisterConfirm string `form:"register_confirm"`
	MailNotify      string `form:"mail_notify"`
	AdminName       string `form:"admin_name" binding:"Required;AlphaDashDot;MaxSize(30)"`
	AdminPasswd     string `form:"admin_pwd" binding:"Required;MinSize(6);MaxSize(255)"`
	ConfirmPasswd   string `form:"confirm_passwd" binding:"Required;MinSize(6);MaxSize(255)"`
	AdminEmail      string `form:"admin_email" binding:"Required;Email;MaxSize(50)"`
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
	Password  string `form:"password" binding:"Required;MinSize(6);MaxSize(255)"`
	Retype    string `form:"retype"`
	LoginType string `form:"logintype"`
	LoginName string `form:"loginname"`
}

func (f *RegisterForm) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}

type SignInForm struct {
	UserName string `form:"uname" binding:"Required;MaxSize(35)"`
	Password string `form:"password" binding:"Required;MinSize(6);MaxSize(255)"`
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
	Email string `form:"email" binding:"Required;Email;MaxSize(50)"`
}

func (f *AddEmailForm) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}

type ChangePasswordForm struct {
	OldPassword string `form:"old_password" binding:"Required;MinSize(6);MaxSize(255)"`
	Password    string `form:"password" binding:"Required;MinSize(6);MaxSize(255)"`
	Retype      string `form:"retype"`
}

func (f *ChangePasswordForm) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}

type AddSSHKeyForm struct {
	SSHTitle string `form:"title" binding:"Required"`
	Content  string `form:"content" binding:"Required"`
}

func (f *AddSSHKeyForm) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}

type NewAccessTokenForm struct {
	Name string `form:"name" binding:"Required"`
}

func (f *NewAccessTokenForm) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}
