// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"fmt"
	"net/url"

	"github.com/go-macaron/captcha"
	log "gopkg.in/clog.v1"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/models/errors"
	"github.com/gogits/gogs/pkg/context"
	"github.com/gogits/gogs/pkg/form"
	"github.com/gogits/gogs/pkg/mailer"
	"github.com/gogits/gogs/pkg/setting"
)

const (
	LOGIN                    = "user/auth/login"
	TWO_FACTOR               = "user/auth/two_factor"
	TWO_FACTOR_RECOVERY_CODE = "user/auth/two_factor_recovery_code"
	SIGNUP                   = "user/auth/signup"
	ACTIVATE                 = "user/auth/activate"
	FORGOT_PASSWORD          = "user/auth/forgot_passwd"
	RESET_PASSWORD           = "user/auth/reset_passwd"
)

// AutoLogin reads cookie and try to auto-login.
func AutoLogin(c *context.Context) (bool, error) {
	if !models.HasEngine {
		return false, nil
	}

	uname := c.GetCookie(setting.CookieUserName)
	if len(uname) == 0 {
		return false, nil
	}

	isSucceed := false
	defer func() {
		if !isSucceed {
			log.Trace("auto-login cookie cleared: %s", uname)
			c.SetCookie(setting.CookieUserName, "", -1, setting.AppSubURL)
			c.SetCookie(setting.CookieRememberName, "", -1, setting.AppSubURL)
			c.SetCookie(setting.LoginStatusCookieName, "", -1, setting.AppSubURL)
		}
	}()

	u, err := models.GetUserByName(uname)
	if err != nil {
		if !errors.IsUserNotExist(err) {
			return false, fmt.Errorf("GetUserByName: %v", err)
		}
		return false, nil
	}

	if val, ok := c.GetSuperSecureCookie(u.Rands+u.Passwd, setting.CookieRememberName); !ok || val != u.Name {
		return false, nil
	}

	isSucceed = true
	c.Session.Set("uid", u.ID)
	c.Session.Set("uname", u.Name)
	c.SetCookie(setting.CSRFCookieName, "", -1, setting.AppSubURL)
	if setting.EnableLoginStatusCookie {
		c.SetCookie(setting.LoginStatusCookieName, "true", 0, setting.AppSubURL)
	}
	return true, nil
}

// isValidRedirect returns false if the URL does not redirect to same site.
// False: //url, http://url
// True: /url
func isValidRedirect(url string) bool {
	return len(url) >= 2 && url[0] == '/' && url[1] != '/'
}

func Login(c *context.Context) {
	c.Data["Title"] = c.Tr("sign_in")

	// Check auto-login.
	isSucceed, err := AutoLogin(c)
	if err != nil {
		c.Handle(500, "AutoLogin", err)
		return
	}

	redirectTo := c.Query("redirect_to")
	if len(redirectTo) > 0 {
		c.SetCookie("redirect_to", redirectTo, 0, setting.AppSubURL)
	} else {
		redirectTo, _ = url.QueryUnescape(c.GetCookie("redirect_to"))
	}
	c.SetCookie("redirect_to", "", -1, setting.AppSubURL)

	if isSucceed {
		if isValidRedirect(redirectTo) {
			c.Redirect(redirectTo)
		} else {
			c.Redirect(setting.AppSubURL + "/")
		}
		return
	}

	c.HTML(200, LOGIN)
}

func afterLogin(c *context.Context, u *models.User, remember bool) {
	if remember {
		days := 86400 * setting.LoginRememberDays
		c.SetCookie(setting.CookieUserName, u.Name, days, setting.AppSubURL, "", setting.CookieSecure, true)
		c.SetSuperSecureCookie(u.Rands+u.Passwd, setting.CookieRememberName, u.Name, days, setting.AppSubURL, "", setting.CookieSecure, true)
	}

	c.Session.Set("uid", u.ID)
	c.Session.Set("uname", u.Name)
	c.Session.Delete("twoFactorRemember")
	c.Session.Delete("twoFactorUserID")

	// Clear whatever CSRF has right now, force to generate a new one
	c.SetCookie(setting.CSRFCookieName, "", -1, setting.AppSubURL)
	if setting.EnableLoginStatusCookie {
		c.SetCookie(setting.LoginStatusCookieName, "true", 0, setting.AppSubURL)
	}

	redirectTo, _ := url.QueryUnescape(c.GetCookie("redirect_to"))
	c.SetCookie("redirect_to", "", -1, setting.AppSubURL)
	if isValidRedirect(redirectTo) {
		c.Redirect(redirectTo)
		return
	}

	c.Redirect(setting.AppSubURL + "/")
}

func LoginPost(c *context.Context, f form.SignIn) {
	c.Data["Title"] = c.Tr("sign_in")

	if c.HasError() {
		c.Success(LOGIN)
		return
	}

	u, err := models.UserSignIn(f.UserName, f.Password)
	if err != nil {
		if errors.IsUserNotExist(err) {
			c.RenderWithErr(c.Tr("form.username_password_incorrect"), LOGIN, &f)
		} else {
			c.ServerError("UserSignIn", err)
		}
		return
	}

	if !u.IsEnabledTwoFactor() {
		afterLogin(c, u, f.Remember)
		return
	}

	c.Session.Set("twoFactorRemember", f.Remember)
	c.Session.Set("twoFactorUserID", u.ID)
	c.Redirect(setting.AppSubURL + "/user/login/two_factor")
}

func LoginTwoFactor(c *context.Context) {
	_, ok := c.Session.Get("twoFactorUserID").(int64)
	if !ok {
		c.NotFound()
		return
	}

	c.Success(TWO_FACTOR)
}

func LoginTwoFactorPost(c *context.Context) {
	userID, ok := c.Session.Get("twoFactorUserID").(int64)
	if !ok {
		c.NotFound()
		return
	}

	t, err := models.GetTwoFactorByUserID(userID)
	if err != nil {
		c.ServerError("GetTwoFactorByUserID", err)
		return
	}
	valid, err := t.ValidateTOTP(c.Query("passcode"))
	if err != nil {
		c.ServerError("ValidateTOTP", err)
		return
	} else if !valid {
		c.Flash.Error(c.Tr("settings.two_factor_invalid_passcode"))
		c.Redirect(setting.AppSubURL + "/user/login/two_factor")
		return
	}

	u, err := models.GetUserByID(userID)
	if err != nil {
		c.ServerError("GetUserByID", err)
		return
	}
	afterLogin(c, u, c.Session.Get("twoFactorRemember").(bool))
}

func LoginTwoFactorRecoveryCode(c *context.Context) {
	_, ok := c.Session.Get("twoFactorUserID").(int64)
	if !ok {
		c.NotFound()
		return
	}

	c.Success(TWO_FACTOR_RECOVERY_CODE)
}

func LoginTwoFactorRecoveryCodePost(c *context.Context) {
	userID, ok := c.Session.Get("twoFactorUserID").(int64)
	if !ok {
		c.NotFound()
		return
	}

	if err := models.UseRecoveryCode(userID, c.Query("recovery_code")); err != nil {
		if errors.IsTwoFactorRecoveryCodeNotFound(err) {
			c.Flash.Error(c.Tr("auth.login_two_factor_invalid_recovery_code"))
			c.Redirect(setting.AppSubURL + "/user/login/two_factor_recovery_code")
		} else {
			c.ServerError("UseRecoveryCode", err)
		}
		return
	}

	u, err := models.GetUserByID(userID)
	if err != nil {
		c.ServerError("GetUserByID", err)
		return
	}
	afterLogin(c, u, c.Session.Get("twoFactorRemember").(bool))
}

func SignOut(ctx *context.Context) {
	ctx.Session.Delete("uid")
	ctx.Session.Delete("uname")
	ctx.SetCookie(setting.CookieUserName, "", -1, setting.AppSubURL)
	ctx.SetCookie(setting.CookieRememberName, "", -1, setting.AppSubURL)
	ctx.SetCookie(setting.CSRFCookieName, "", -1, setting.AppSubURL)
	ctx.Redirect(setting.AppSubURL + "/")
}

func SignUp(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("sign_up")

	ctx.Data["EnableCaptcha"] = setting.Service.EnableCaptcha

	if setting.Service.DisableRegistration {
		ctx.Data["DisableRegistration"] = true
		ctx.HTML(200, SIGNUP)
		return
	}

	ctx.HTML(200, SIGNUP)
}

func SignUpPost(ctx *context.Context, cpt *captcha.Captcha, f form.Register) {
	ctx.Data["Title"] = ctx.Tr("sign_up")

	ctx.Data["EnableCaptcha"] = setting.Service.EnableCaptcha

	if setting.Service.DisableRegistration {
		ctx.Error(403)
		return
	}

	if ctx.HasError() {
		ctx.HTML(200, SIGNUP)
		return
	}

	if setting.Service.EnableCaptcha && !cpt.VerifyReq(ctx.Req) {
		ctx.Data["Err_Captcha"] = true
		ctx.RenderWithErr(ctx.Tr("form.captcha_incorrect"), SIGNUP, &f)
		return
	}

	if f.Password != f.Retype {
		ctx.Data["Err_Password"] = true
		ctx.RenderWithErr(ctx.Tr("form.password_not_match"), SIGNUP, &f)
		return
	}

	u := &models.User{
		Name:     f.UserName,
		Email:    f.Email,
		Passwd:   f.Password,
		IsActive: !setting.Service.RegisterEmailConfirm,
	}
	if err := models.CreateUser(u); err != nil {
		switch {
		case models.IsErrUserAlreadyExist(err):
			ctx.Data["Err_UserName"] = true
			ctx.RenderWithErr(ctx.Tr("form.username_been_taken"), SIGNUP, &f)
		case models.IsErrEmailAlreadyUsed(err):
			ctx.Data["Err_Email"] = true
			ctx.RenderWithErr(ctx.Tr("form.email_been_used"), SIGNUP, &f)
		case models.IsErrNameReserved(err):
			ctx.Data["Err_UserName"] = true
			ctx.RenderWithErr(ctx.Tr("user.form.name_reserved", err.(models.ErrNameReserved).Name), SIGNUP, &f)
		case models.IsErrNamePatternNotAllowed(err):
			ctx.Data["Err_UserName"] = true
			ctx.RenderWithErr(ctx.Tr("user.form.name_pattern_not_allowed", err.(models.ErrNamePatternNotAllowed).Pattern), SIGNUP, &f)
		default:
			ctx.Handle(500, "CreateUser", err)
		}
		return
	}
	log.Trace("Account created: %s", u.Name)

	// Auto-set admin for the only user.
	if models.CountUsers() == 1 {
		u.IsAdmin = true
		u.IsActive = true
		if err := models.UpdateUser(u); err != nil {
			ctx.Handle(500, "UpdateUser", err)
			return
		}
	}

	// Send confirmation email, no need for social account.
	if setting.Service.RegisterEmailConfirm && u.ID > 1 {
		mailer.SendActivateAccountMail(ctx.Context, models.NewMailerUser(u))
		ctx.Data["IsSendRegisterMail"] = true
		ctx.Data["Email"] = u.Email
		ctx.Data["Hours"] = setting.Service.ActiveCodeLives / 60
		ctx.HTML(200, ACTIVATE)

		if err := ctx.Cache.Put("MailResendLimit_"+u.LowerName, u.LowerName, 180); err != nil {
			log.Error(4, "Set cache(MailResendLimit) fail: %v", err)
		}
		return
	}

	ctx.Redirect(setting.AppSubURL + "/user/login")
}

func Activate(ctx *context.Context) {
	code := ctx.Query("code")
	if len(code) == 0 {
		ctx.Data["IsActivatePage"] = true
		if ctx.User.IsActive {
			ctx.Error(404)
			return
		}
		// Resend confirmation email.
		if setting.Service.RegisterEmailConfirm {
			if ctx.Cache.IsExist("MailResendLimit_" + ctx.User.LowerName) {
				ctx.Data["ResendLimited"] = true
			} else {
				ctx.Data["Hours"] = setting.Service.ActiveCodeLives / 60
				mailer.SendActivateAccountMail(ctx.Context, models.NewMailerUser(ctx.User))

				keyName := "MailResendLimit_" + ctx.User.LowerName
				if err := ctx.Cache.Put(keyName, ctx.User.LowerName, 180); err != nil {
					log.Error(2, "Set cache '%s' fail: %v", keyName, err)
				}
			}
		} else {
			ctx.Data["ServiceNotEnabled"] = true
		}
		ctx.HTML(200, ACTIVATE)
		return
	}

	// Verify code.
	if user := models.VerifyUserActiveCode(code); user != nil {
		user.IsActive = true
		var err error
		if user.Rands, err = models.GetUserSalt(); err != nil {
			ctx.Handle(500, "UpdateUser", err)
			return
		}
		if err := models.UpdateUser(user); err != nil {
			ctx.Handle(500, "UpdateUser", err)
			return
		}

		log.Trace("User activated: %s", user.Name)

		ctx.Session.Set("uid", user.ID)
		ctx.Session.Set("uname", user.Name)
		ctx.Redirect(setting.AppSubURL + "/")
		return
	}

	ctx.Data["IsActivateFailed"] = true
	ctx.HTML(200, ACTIVATE)
}

func ActivateEmail(ctx *context.Context) {
	code := ctx.Query("code")
	email_string := ctx.Query("email")

	// Verify code.
	if email := models.VerifyActiveEmailCode(code, email_string); email != nil {
		if err := email.Activate(); err != nil {
			ctx.Handle(500, "ActivateEmail", err)
		}

		log.Trace("Email activated: %s", email.Email)
		ctx.Flash.Success(ctx.Tr("settings.add_email_success"))
	}

	ctx.Redirect(setting.AppSubURL + "/user/settings/email")
	return
}

func ForgotPasswd(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("auth.forgot_password")

	if setting.MailService == nil {
		ctx.Data["IsResetDisable"] = true
		ctx.HTML(200, FORGOT_PASSWORD)
		return
	}

	ctx.Data["IsResetRequest"] = true
	ctx.HTML(200, FORGOT_PASSWORD)
}

func ForgotPasswdPost(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("auth.forgot_password")

	if setting.MailService == nil {
		ctx.Handle(403, "ForgotPasswdPost", nil)
		return
	}
	ctx.Data["IsResetRequest"] = true

	email := ctx.Query("email")
	ctx.Data["Email"] = email

	u, err := models.GetUserByEmail(email)
	if err != nil {
		if errors.IsUserNotExist(err) {
			ctx.Data["Hours"] = setting.Service.ActiveCodeLives / 60
			ctx.Data["IsResetSent"] = true
			ctx.HTML(200, FORGOT_PASSWORD)
			return
		} else {
			ctx.Handle(500, "user.ResetPasswd(check existence)", err)
		}
		return
	}

	if !u.IsLocal() {
		ctx.Data["Err_Email"] = true
		ctx.RenderWithErr(ctx.Tr("auth.non_local_account"), FORGOT_PASSWORD, nil)
		return
	}

	if ctx.Cache.IsExist("MailResendLimit_" + u.LowerName) {
		ctx.Data["ResendLimited"] = true
		ctx.HTML(200, FORGOT_PASSWORD)
		return
	}

	mailer.SendResetPasswordMail(ctx.Context, models.NewMailerUser(u))
	if err = ctx.Cache.Put("MailResendLimit_"+u.LowerName, u.LowerName, 180); err != nil {
		log.Error(4, "Set cache(MailResendLimit) fail: %v", err)
	}

	ctx.Data["Hours"] = setting.Service.ActiveCodeLives / 60
	ctx.Data["IsResetSent"] = true
	ctx.HTML(200, FORGOT_PASSWORD)
}

func ResetPasswd(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("auth.reset_password")

	code := ctx.Query("code")
	if len(code) == 0 {
		ctx.Error(404)
		return
	}
	ctx.Data["Code"] = code
	ctx.Data["IsResetForm"] = true
	ctx.HTML(200, RESET_PASSWORD)
}

func ResetPasswdPost(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("auth.reset_password")

	code := ctx.Query("code")
	if len(code) == 0 {
		ctx.Error(404)
		return
	}
	ctx.Data["Code"] = code

	if u := models.VerifyUserActiveCode(code); u != nil {
		// Validate password length.
		passwd := ctx.Query("password")
		if len(passwd) < 6 {
			ctx.Data["IsResetForm"] = true
			ctx.Data["Err_Password"] = true
			ctx.RenderWithErr(ctx.Tr("auth.password_too_short"), RESET_PASSWORD, nil)
			return
		}

		u.Passwd = passwd
		var err error
		if u.Rands, err = models.GetUserSalt(); err != nil {
			ctx.Handle(500, "UpdateUser", err)
			return
		}
		if u.Salt, err = models.GetUserSalt(); err != nil {
			ctx.Handle(500, "UpdateUser", err)
			return
		}
		u.EncodePasswd()
		if err := models.UpdateUser(u); err != nil {
			ctx.Handle(500, "UpdateUser", err)
			return
		}

		log.Trace("User password reset: %s", u.Name)
		ctx.Redirect(setting.AppSubURL + "/user/login")
		return
	}

	ctx.Data["IsResetFailed"] = true
	ctx.HTML(200, RESET_PASSWORD)
}
