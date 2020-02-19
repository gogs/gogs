// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"fmt"
	"net/url"

	"github.com/go-macaron/captcha"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/db/errors"
	"gogs.io/gogs/internal/form"
	"gogs.io/gogs/internal/mailer"
	"gogs.io/gogs/internal/setting"
	"gogs.io/gogs/internal/tool"
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
	if !db.HasEngine {
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

	u, err := db.GetUserByName(uname)
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

func Login(c *context.Context) {
	c.Title("sign_in")

	// Check auto-login
	isSucceed, err := AutoLogin(c)
	if err != nil {
		c.ServerError("AutoLogin", err)
		return
	}

	redirectTo := c.Query("redirect_to")
	if len(redirectTo) > 0 {
		c.SetCookie("redirect_to", redirectTo, 0, setting.AppSubURL)
	} else {
		redirectTo, _ = url.QueryUnescape(c.GetCookie("redirect_to"))
	}

	if isSucceed {
		if tool.IsSameSiteURLPath(redirectTo) {
			c.Redirect(redirectTo)
		} else {
			c.SubURLRedirect("/")
		}
		c.SetCookie("redirect_to", "", -1, setting.AppSubURL)
		return
	}

	// Display normal login page
	loginSources, err := db.ActivatedLoginSources()
	if err != nil {
		c.ServerError("ActivatedLoginSources", err)
		return
	}
	c.Data["LoginSources"] = loginSources
	for i := range loginSources {
		if loginSources[i].IsDefault {
			c.Data["DefaultLoginSource"] = loginSources[i]
			c.Data["login_source"] = loginSources[i].ID
			break
		}
	}
	c.Success(LOGIN)
}

func afterLogin(c *context.Context, u *db.User, remember bool) {
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
	if tool.IsSameSiteURLPath(redirectTo) {
		c.Redirect(redirectTo)
		return
	}

	c.SubURLRedirect("/")
}

func LoginPost(c *context.Context, f form.SignIn) {
	c.Title("sign_in")

	loginSources, err := db.ActivatedLoginSources()
	if err != nil {
		c.ServerError("ActivatedLoginSources", err)
		return
	}
	c.Data["LoginSources"] = loginSources

	if c.HasError() {
		c.Success(LOGIN)
		return
	}

	u, err := db.UserLogin(f.UserName, f.Password, f.LoginSource)
	if err != nil {
		switch err.(type) {
		case errors.UserNotExist:
			c.FormErr("UserName", "Password")
			c.RenderWithErr(c.Tr("form.username_password_incorrect"), LOGIN, &f)
		case errors.LoginSourceMismatch:
			c.FormErr("LoginSource")
			c.RenderWithErr(c.Tr("form.auth_source_mismatch"), LOGIN, &f)

		default:
			c.ServerError("UserLogin", err)
		}
		for i := range loginSources {
			if loginSources[i].IsDefault {
				c.Data["DefaultLoginSource"] = loginSources[i]
				break
			}
		}
		return
	}

	if !u.IsEnabledTwoFactor() {
		afterLogin(c, u, f.Remember)
		return
	}

	c.Session.Set("twoFactorRemember", f.Remember)
	c.Session.Set("twoFactorUserID", u.ID)
	c.SubURLRedirect("/user/login/two_factor")
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

	t, err := db.GetTwoFactorByUserID(userID)
	if err != nil {
		c.ServerError("GetTwoFactorByUserID", err)
		return
	}

	passcode := c.Query("passcode")
	valid, err := t.ValidateTOTP(passcode)
	if err != nil {
		c.ServerError("ValidateTOTP", err)
		return
	} else if !valid {
		c.Flash.Error(c.Tr("settings.two_factor_invalid_passcode"))
		c.SubURLRedirect("/user/login/two_factor")
		return
	}

	u, err := db.GetUserByID(userID)
	if err != nil {
		c.ServerError("GetUserByID", err)
		return
	}

	// Prevent same passcode from being reused
	if c.Cache.IsExist(u.TwoFactorCacheKey(passcode)) {
		c.Flash.Error(c.Tr("settings.two_factor_reused_passcode"))
		c.SubURLRedirect("/user/login/two_factor")
		return
	}
	if err = c.Cache.Put(u.TwoFactorCacheKey(passcode), 1, 60); err != nil {
		log.Error("Failed to put cache 'two factor passcode': %v", err)
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

	if err := db.UseRecoveryCode(userID, c.Query("recovery_code")); err != nil {
		if errors.IsTwoFactorRecoveryCodeNotFound(err) {
			c.Flash.Error(c.Tr("auth.login_two_factor_invalid_recovery_code"))
			c.SubURLRedirect("/user/login/two_factor_recovery_code")
		} else {
			c.ServerError("UseRecoveryCode", err)
		}
		return
	}

	u, err := db.GetUserByID(userID)
	if err != nil {
		c.ServerError("GetUserByID", err)
		return
	}
	afterLogin(c, u, c.Session.Get("twoFactorRemember").(bool))
}

func SignOut(c *context.Context) {
	c.Session.Flush()
	c.Session.Destory(c.Context)
	c.SetCookie(setting.CookieUserName, "", -1, setting.AppSubURL)
	c.SetCookie(setting.CookieRememberName, "", -1, setting.AppSubURL)
	c.SetCookie(setting.CSRFCookieName, "", -1, setting.AppSubURL)
	c.SubURLRedirect("/")
}

func SignUp(c *context.Context) {
	c.Title("sign_up")

	c.Data["EnableCaptcha"] = setting.Service.EnableCaptcha

	if setting.Service.DisableRegistration {
		c.Data["DisableRegistration"] = true
		c.Success(SIGNUP)
		return
	}

	c.Success(SIGNUP)
}

func SignUpPost(c *context.Context, cpt *captcha.Captcha, f form.Register) {
	c.Title("sign_up")

	c.Data["EnableCaptcha"] = setting.Service.EnableCaptcha

	if setting.Service.DisableRegistration {
		c.Status(403)
		return
	}

	if c.HasError() {
		c.Success(SIGNUP)
		return
	}

	if setting.Service.EnableCaptcha && !cpt.VerifyReq(c.Req) {
		c.FormErr("Captcha")
		c.RenderWithErr(c.Tr("form.captcha_incorrect"), SIGNUP, &f)
		return
	}

	if f.Password != f.Retype {
		c.FormErr("Password")
		c.RenderWithErr(c.Tr("form.password_not_match"), SIGNUP, &f)
		return
	}

	u := &db.User{
		Name:     f.UserName,
		Email:    f.Email,
		Passwd:   f.Password,
		IsActive: !setting.Service.RegisterEmailConfirm,
	}
	if err := db.CreateUser(u); err != nil {
		switch {
		case db.IsErrUserAlreadyExist(err):
			c.FormErr("UserName")
			c.RenderWithErr(c.Tr("form.username_been_taken"), SIGNUP, &f)
		case db.IsErrEmailAlreadyUsed(err):
			c.FormErr("Email")
			c.RenderWithErr(c.Tr("form.email_been_used"), SIGNUP, &f)
		case db.IsErrNameReserved(err):
			c.FormErr("UserName")
			c.RenderWithErr(c.Tr("user.form.name_reserved", err.(db.ErrNameReserved).Name), SIGNUP, &f)
		case db.IsErrNamePatternNotAllowed(err):
			c.FormErr("UserName")
			c.RenderWithErr(c.Tr("user.form.name_pattern_not_allowed", err.(db.ErrNamePatternNotAllowed).Pattern), SIGNUP, &f)
		default:
			c.ServerError("CreateUser", err)
		}
		return
	}
	log.Trace("Account created: %s", u.Name)

	// Auto-set admin for the only user.
	if db.CountUsers() == 1 {
		u.IsAdmin = true
		u.IsActive = true
		if err := db.UpdateUser(u); err != nil {
			c.ServerError("UpdateUser", err)
			return
		}
	}

	// Send confirmation email, no need for social account.
	if setting.Service.RegisterEmailConfirm && u.ID > 1 {
		mailer.SendActivateAccountMail(c.Context, db.NewMailerUser(u))
		c.Data["IsSendRegisterMail"] = true
		c.Data["Email"] = u.Email
		c.Data["Hours"] = setting.Service.ActiveCodeLives / 60
		c.Success(ACTIVATE)

		if err := c.Cache.Put(u.MailResendCacheKey(), 1, 180); err != nil {
			log.Error("Failed to put cache key 'mail resend': %v", err)
		}
		return
	}

	c.SubURLRedirect("/user/login")
}

func Activate(c *context.Context) {
	code := c.Query("code")
	if len(code) == 0 {
		c.Data["IsActivatePage"] = true
		if c.User.IsActive {
			c.NotFound()
			return
		}
		// Resend confirmation email.
		if setting.Service.RegisterEmailConfirm {
			if c.Cache.IsExist(c.User.MailResendCacheKey()) {
				c.Data["ResendLimited"] = true
			} else {
				c.Data["Hours"] = setting.Service.ActiveCodeLives / 60
				mailer.SendActivateAccountMail(c.Context, db.NewMailerUser(c.User))

				if err := c.Cache.Put(c.User.MailResendCacheKey(), 1, 180); err != nil {
					log.Error("Failed to put cache key 'mail resend': %v", err)
				}
			}
		} else {
			c.Data["ServiceNotEnabled"] = true
		}
		c.Success(ACTIVATE)
		return
	}

	// Verify code.
	if user := db.VerifyUserActiveCode(code); user != nil {
		user.IsActive = true
		var err error
		if user.Rands, err = db.GetUserSalt(); err != nil {
			c.ServerError("GetUserSalt", err)
			return
		}
		if err := db.UpdateUser(user); err != nil {
			c.ServerError("UpdateUser", err)
			return
		}

		log.Trace("User activated: %s", user.Name)

		c.Session.Set("uid", user.ID)
		c.Session.Set("uname", user.Name)
		c.SubURLRedirect("/")
		return
	}

	c.Data["IsActivateFailed"] = true
	c.Success(ACTIVATE)
}

func ActivateEmail(c *context.Context) {
	code := c.Query("code")
	email_string := c.Query("email")

	// Verify code.
	if email := db.VerifyActiveEmailCode(code, email_string); email != nil {
		if err := email.Activate(); err != nil {
			c.ServerError("ActivateEmail", err)
		}

		log.Trace("Email activated: %s", email.Email)
		c.Flash.Success(c.Tr("settings.add_email_success"))
	}

	c.SubURLRedirect("/user/settings/email")
	return
}

func ForgotPasswd(c *context.Context) {
	c.Title("auth.forgot_password")

	if setting.MailService == nil {
		c.Data["IsResetDisable"] = true
		c.Success(FORGOT_PASSWORD)
		return
	}

	c.Data["IsResetRequest"] = true
	c.Success(FORGOT_PASSWORD)
}

func ForgotPasswdPost(c *context.Context) {
	c.Title("auth.forgot_password")

	if setting.MailService == nil {
		c.Status(403)
		return
	}
	c.Data["IsResetRequest"] = true

	email := c.Query("email")
	c.Data["Email"] = email

	u, err := db.GetUserByEmail(email)
	if err != nil {
		if errors.IsUserNotExist(err) {
			c.Data["Hours"] = setting.Service.ActiveCodeLives / 60
			c.Data["IsResetSent"] = true
			c.Success(FORGOT_PASSWORD)
			return
		} else {
			c.ServerError("GetUserByEmail", err)
		}
		return
	}

	if !u.IsLocal() {
		c.FormErr("Email")
		c.RenderWithErr(c.Tr("auth.non_local_account"), FORGOT_PASSWORD, nil)
		return
	}

	if c.Cache.IsExist(u.MailResendCacheKey()) {
		c.Data["ResendLimited"] = true
		c.Success(FORGOT_PASSWORD)
		return
	}

	mailer.SendResetPasswordMail(c.Context, db.NewMailerUser(u))
	if err = c.Cache.Put(u.MailResendCacheKey(), 1, 180); err != nil {
		log.Error("Failed to put cache key 'mail resend': %v", err)
	}

	c.Data["Hours"] = setting.Service.ActiveCodeLives / 60
	c.Data["IsResetSent"] = true
	c.Success(FORGOT_PASSWORD)
}

func ResetPasswd(c *context.Context) {
	c.Title("auth.reset_password")

	code := c.Query("code")
	if len(code) == 0 {
		c.NotFound()
		return
	}
	c.Data["Code"] = code
	c.Data["IsResetForm"] = true
	c.Success(RESET_PASSWORD)
}

func ResetPasswdPost(c *context.Context) {
	c.Title("auth.reset_password")

	code := c.Query("code")
	if len(code) == 0 {
		c.NotFound()
		return
	}
	c.Data["Code"] = code

	if u := db.VerifyUserActiveCode(code); u != nil {
		// Validate password length.
		passwd := c.Query("password")
		if len(passwd) < 6 {
			c.Data["IsResetForm"] = true
			c.Data["Err_Password"] = true
			c.RenderWithErr(c.Tr("auth.password_too_short"), RESET_PASSWORD, nil)
			return
		}

		u.Passwd = passwd
		var err error
		if u.Rands, err = db.GetUserSalt(); err != nil {
			c.ServerError("GetUserSalt", err)
			return
		}
		if u.Salt, err = db.GetUserSalt(); err != nil {
			c.ServerError("GetUserSalt", err)
			return
		}
		u.EncodePasswd()
		if err := db.UpdateUser(u); err != nil {
			c.ServerError("UpdateUser", err)
			return
		}

		log.Trace("User password reset: %s", u.Name)
		c.SubURLRedirect("/user/login")
		return
	}

	c.Data["IsResetFailed"] = true
	c.Success(RESET_PASSWORD)
}
