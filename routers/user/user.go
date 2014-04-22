// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"net/url"
	"strings"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/mailer"
	"github.com/gogits/gogs/modules/middleware"
)

func SignIn(ctx *middleware.Context) {
	ctx.Data["Title"] = "Log In"

	if _, ok := ctx.Session.Get("socialId").(int64); ok {
		ctx.Data["IsSocialLogin"] = true
		ctx.HTML(200, "user/signin")
		return
	}

	if base.OauthService != nil {
		ctx.Data["OauthEnabled"] = true
		ctx.Data["OauthService"] = base.OauthService
	}

	// Check auto-login.
	userName := ctx.GetCookie(base.CookieUserName)
	if len(userName) == 0 {
		ctx.HTML(200, "user/signin")
		return
	}

	isSucceed := false
	defer func() {
		if !isSucceed {
			log.Trace("user.SignIn(auto-login cookie cleared): %s", userName)
			ctx.SetCookie(base.CookieUserName, "", -1)
			ctx.SetCookie(base.CookieRememberName, "", -1)
			return
		}
	}()

	user, err := models.GetUserByName(userName)
	if err != nil {
		ctx.HTML(500, "user/signin")
		return
	}

	secret := base.EncodeMd5(user.Rands + user.Passwd)
	value, _ := ctx.GetSecureCookie(secret, base.CookieRememberName)
	if value != user.Name {
		ctx.HTML(500, "user/signin")
		return
	}

	isSucceed = true

	ctx.Session.Set("userId", user.Id)
	ctx.Session.Set("userName", user.Name)
	if redirectTo, _ := url.QueryUnescape(ctx.GetCookie("redirect_to")); len(redirectTo) > 0 {
		ctx.SetCookie("redirect_to", "", -1)
		ctx.Redirect(redirectTo)
		return
	}

	ctx.Redirect("/")
}

func SignInPost(ctx *middleware.Context, form auth.LogInForm) {
	ctx.Data["Title"] = "Log In"

	sid, isOauth := ctx.Session.Get("socialId").(int64)
	if isOauth {
		ctx.Data["IsSocialLogin"] = true
	} else if base.OauthService != nil {
		ctx.Data["OauthEnabled"] = true
		ctx.Data["OauthService"] = base.OauthService
	}

	if ctx.HasError() {
		ctx.HTML(200, "user/signin")
		return
	}

	user, err := models.LoginUserPlain(form.UserName, form.Password)
	if err != nil {
		if err == models.ErrUserNotExist {
			log.Trace("%s Log in failed: %s/%s", ctx.Req.RequestURI, form.UserName, form.Password)
			ctx.RenderWithErr("Username or password is not correct", "user/signin", &form)
			return
		}

		ctx.Handle(500, "user.SignIn", err)
		return
	}

	if form.Remember == "on" {
		secret := base.EncodeMd5(user.Rands + user.Passwd)
		days := 86400 * base.LogInRememberDays
		ctx.SetCookie(base.CookieUserName, user.Name, days)
		ctx.SetSecureCookie(secret, base.CookieRememberName, user.Name, days)
	}

	// Bind with social account.
	if isOauth {
		if err = models.BindUserOauth2(user.Id, sid); err != nil {
			if err == models.ErrOauth2RecordNotExist {
				ctx.Handle(404, "user.SignInPost(GetOauth2ById)", err)
			} else {
				ctx.Handle(500, "user.SignInPost(GetOauth2ById)", err)
			}
			return
		}
		ctx.Session.Delete("socialId")
		log.Trace("%s OAuth binded: %s -> %d", ctx.Req.RequestURI, form.UserName, sid)
	}

	ctx.Session.Set("userId", user.Id)
	ctx.Session.Set("userName", user.Name)
	if redirectTo, _ := url.QueryUnescape(ctx.GetCookie("redirect_to")); len(redirectTo) > 0 {
		ctx.SetCookie("redirect_to", "", -1)
		ctx.Redirect(redirectTo)
		return
	}

	ctx.Redirect("/")
}

func oauthSignInPost(ctx *middleware.Context, sid int64) {
	ctx.Data["Title"] = "OAuth Sign Up"
	ctx.Data["PageIsSignUp"] = true

	if _, err := models.GetOauth2ById(sid); err != nil {
		if err == models.ErrOauth2RecordNotExist {
			ctx.Handle(404, "user.oauthSignUp(GetOauth2ById)", err)
		} else {
			ctx.Handle(500, "user.oauthSignUp(GetOauth2ById)", err)
		}
		return
	}

	ctx.Data["IsSocialLogin"] = true
	ctx.Data["username"] = ctx.Session.Get("socialName")
	ctx.Data["email"] = ctx.Session.Get("socialEmail")
	log.Trace("user.oauthSignUp(social ID): %v", ctx.Session.Get("socialId"))

	ctx.HTML(200, "user/signup")
}

func SignOut(ctx *middleware.Context) {
	ctx.Session.Delete("userId")
	ctx.Session.Delete("userName")
	ctx.Session.Delete("socialId")
	ctx.Session.Delete("socialName")
	ctx.Session.Delete("socialEmail")
	ctx.SetCookie(base.CookieUserName, "", -1)
	ctx.SetCookie(base.CookieRememberName, "", -1)
	ctx.Redirect("/")
}

func SignUp(ctx *middleware.Context) {
	ctx.Data["Title"] = "Sign Up"
	ctx.Data["PageIsSignUp"] = true

	if base.Service.DisableRegistration {
		ctx.Data["DisableRegistration"] = true
		ctx.HTML(200, "user/signup")
		return
	}

	if sid, ok := ctx.Session.Get("socialId").(int64); ok {
		oauthSignUp(ctx, sid)
		return
	}

	ctx.HTML(200, "user/signup")
}

func oauthSignUp(ctx *middleware.Context, sid int64) {
	ctx.Data["Title"] = "OAuth Sign Up"
	ctx.Data["PageIsSignUp"] = true

	if _, err := models.GetOauth2ById(sid); err != nil {
		if err == models.ErrOauth2RecordNotExist {
			ctx.Handle(404, "user.oauthSignUp(GetOauth2ById)", err)
		} else {
			ctx.Handle(500, "user.oauthSignUp(GetOauth2ById)", err)
		}
		return
	}

	ctx.Data["IsSocialLogin"] = true
	ctx.Data["username"] = strings.Replace(ctx.Session.Get("socialName").(string), " ", "", -1)
	ctx.Data["email"] = ctx.Session.Get("socialEmail")
	log.Trace("user.oauthSignUp(social ID): %v", ctx.Session.Get("socialId"))

	ctx.HTML(200, "user/signup")
}

func SignUpPost(ctx *middleware.Context, form auth.RegisterForm) {
	ctx.Data["Title"] = "Sign Up"
	ctx.Data["PageIsSignUp"] = true

	if base.Service.DisableRegistration {
		ctx.Handle(403, "user.SignUpPost", nil)
		return
	}

	sid, isOauth := ctx.Session.Get("socialId").(int64)
	if isOauth {
		ctx.Data["IsSocialLogin"] = true
	}

	if form.Password != form.RetypePasswd {
		ctx.Data["HasError"] = true
		ctx.Data["Err_Password"] = true
		ctx.Data["Err_RetypePasswd"] = true
		ctx.Data["ErrorMsg"] = "Password and re-type password are not same"
		auth.AssignForm(form, ctx.Data)
	}

	if ctx.HasError() {
		ctx.HTML(200, "user/signup")
		return
	}

	u := &models.User{
		Name:     form.UserName,
		Email:    form.Email,
		Passwd:   form.Password,
		IsActive: !base.Service.RegisterEmailConfirm || isOauth,
	}

	var err error
	if u, err = models.RegisterUser(u); err != nil {
		switch err {
		case models.ErrUserAlreadyExist:
			ctx.RenderWithErr("Username has been already taken", "user/signup", &form)
		case models.ErrEmailAlreadyUsed:
			ctx.RenderWithErr("E-mail address has been already used", "user/signup", &form)
		case models.ErrUserNameIllegal:
			ctx.RenderWithErr(models.ErrRepoNameIllegal.Error(), "user/signup", &form)
		default:
			ctx.Handle(500, "user.SignUp(RegisterUser)", err)
		}
		return
	}

	log.Trace("%s User created: %s", ctx.Req.RequestURI, form.UserName)

	// Bind social account.
	if isOauth {
		if err = models.BindUserOauth2(u.Id, sid); err != nil {
			ctx.Handle(500, "user.SignUp(BindUserOauth2)", err)
			return
		}
		ctx.Session.Delete("socialId")
		log.Trace("%s OAuth binded: %s -> %d", ctx.Req.RequestURI, form.UserName, sid)
	}

	// Send confirmation e-mail, no need for social account.
	if !isOauth && base.Service.RegisterEmailConfirm && u.Id > 1 {
		mailer.SendRegisterMail(ctx.Render, u)
		ctx.Data["IsSendRegisterMail"] = true
		ctx.Data["Email"] = u.Email
		ctx.Data["Hours"] = base.Service.ActiveCodeLives / 60
		ctx.HTML(200, "user/activate")

		if err = ctx.Cache.Put("MailResendLimit_"+u.LowerName, u.LowerName, 180); err != nil {
			log.Error("Set cache(MailResendLimit) fail: %v", err)
		}
		return
	}
	ctx.Redirect("/user/login")
}

func Delete(ctx *middleware.Context) {
	ctx.Data["Title"] = "Delete Account"
	ctx.Data["PageIsUserSetting"] = true
	ctx.Data["IsUserPageSettingDelete"] = true
	ctx.HTML(200, "user/delete")
}

func DeletePost(ctx *middleware.Context) {
	ctx.Data["Title"] = "Delete Account"
	ctx.Data["PageIsUserSetting"] = true
	ctx.Data["IsUserPageSettingDelete"] = true

	tmpUser := models.User{
		Passwd: ctx.Query("password"),
		Salt:   ctx.User.Salt,
	}
	tmpUser.EncodePasswd()
	if tmpUser.Passwd != ctx.User.Passwd {
		ctx.Flash.Error("Password is not correct. Make sure you are owner of this account.")
	} else {
		if err := models.DeleteUser(ctx.User); err != nil {
			switch err {
			case models.ErrUserOwnRepos:
				ctx.Flash.Error("Your account still have ownership of repository, you have to delete or transfer them first.")
			default:
				ctx.Handle(500, "user.Delete", err)
				return
			}
		} else {
			ctx.Redirect("/")
			return
		}
	}

	ctx.Redirect("/user/delete")
}

func Activate(ctx *middleware.Context) {
	code := ctx.Query("code")
	if len(code) == 0 {
		ctx.Data["IsActivatePage"] = true
		if ctx.User.IsActive {
			ctx.Handle(404, "user.Activate", nil)
			return
		}
		// Resend confirmation e-mail.
		if base.Service.RegisterEmailConfirm {
			if ctx.Cache.IsExist("MailResendLimit_" + ctx.User.LowerName) {
				ctx.Data["ResendLimited"] = true
			} else {
				ctx.Data["Hours"] = base.Service.ActiveCodeLives / 60
				mailer.SendActiveMail(ctx.Render, ctx.User)

				if err := ctx.Cache.Put("MailResendLimit_"+ctx.User.LowerName, ctx.User.LowerName, 180); err != nil {
					log.Error("Set cache(MailResendLimit) fail: %v", err)
				}
			}
		} else {
			ctx.Data["ServiceNotEnabled"] = true
		}
		ctx.HTML(200, "user/activate")
		return
	}

	// Verify code.
	if user := models.VerifyUserActiveCode(code); user != nil {
		user.IsActive = true
		user.Rands = models.GetUserSalt()
		if err := models.UpdateUser(user); err != nil {
			ctx.Handle(404, "user.Activate", err)
			return
		}

		log.Trace("%s User activated: %s", ctx.Req.RequestURI, user.Name)

		ctx.Session.Set("userId", user.Id)
		ctx.Session.Set("userName", user.Name)
		ctx.Redirect("/")
		return
	}

	ctx.Data["IsActivateFailed"] = true
	ctx.HTML(200, "user/activate")
}

func ForgotPasswd(ctx *middleware.Context) {
	ctx.Data["Title"] = "Forgot Password"

	if base.MailService == nil {
		ctx.Data["IsResetDisable"] = true
		ctx.HTML(200, "user/forgot_passwd")
		return
	}

	ctx.Data["IsResetRequest"] = true
	ctx.HTML(200, "user/forgot_passwd")
}

func ForgotPasswdPost(ctx *middleware.Context) {
	ctx.Data["Title"] = "Forgot Password"

	if base.MailService == nil {
		ctx.Handle(403, "user.ForgotPasswdPost", nil)
		return
	}
	ctx.Data["IsResetRequest"] = true

	email := ctx.Query("email")
	u, err := models.GetUserByEmail(email)
	if err != nil {
		if err == models.ErrUserNotExist {
			ctx.RenderWithErr("This e-mail address does not associate to any account.", "user/forgot_passwd", nil)
		} else {
			ctx.Handle(500, "user.ResetPasswd(check existence)", err)
		}
		return
	}

	if ctx.Cache.IsExist("MailResendLimit_" + u.LowerName) {
		ctx.Data["ResendLimited"] = true
		ctx.HTML(200, "user/forgot_passwd")
		return
	}

	mailer.SendResetPasswdMail(ctx.Render, u)
	if err = ctx.Cache.Put("MailResendLimit_"+u.LowerName, u.LowerName, 180); err != nil {
		log.Error("Set cache(MailResendLimit) fail: %v", err)
	}

	ctx.Data["Email"] = email
	ctx.Data["Hours"] = base.Service.ActiveCodeLives / 60
	ctx.Data["IsResetSent"] = true
	ctx.HTML(200, "user/forgot_passwd")
}

func ResetPasswd(ctx *middleware.Context) {
	ctx.Data["Title"] = "Reset Password"

	code := ctx.Query("code")
	if len(code) == 0 {
		ctx.Error(404)
		return
	}
	ctx.Data["Code"] = code

	ctx.Data["IsResetForm"] = true
	ctx.HTML(200, "user/reset_passwd")
}

func ResetPasswdPost(ctx *middleware.Context) {
	ctx.Data["Title"] = "Reset Password"

	code := ctx.Query("code")
	if len(code) == 0 {
		ctx.Error(404)
		return
	}
	ctx.Data["Code"] = code

	if u := models.VerifyUserActiveCode(code); u != nil {
		// Validate password length.
		passwd := ctx.Query("passwd")
		if len(passwd) < 6 || len(passwd) > 30 {
			ctx.Data["IsResetForm"] = true
			ctx.RenderWithErr("Password length should be in 6 and 30.", "user/reset_passwd", nil)
			return
		}

		u.Passwd = passwd
		u.Rands = models.GetUserSalt()
		u.Salt = models.GetUserSalt()
		u.EncodePasswd()
		if err := models.UpdateUser(u); err != nil {
			ctx.Handle(500, "user.ResetPasswd(UpdateUser)", err)
			return
		}

		log.Trace("%s User password reset: %s", ctx.Req.RequestURI, u.Name)
		ctx.Redirect("/user/login")
		return
	}

	ctx.Data["IsResetFailed"] = true
	ctx.HTML(200, "user/reset_passwd")
}
