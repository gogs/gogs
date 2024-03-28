// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	gocontext "context"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"

	"github.com/go-macaron/captcha"
	"github.com/unknwon/com"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/email"
	"gogs.io/gogs/internal/form"
	"gogs.io/gogs/internal/tool"
	"gogs.io/gogs/internal/userutil"
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
	if !database.HasEngine {
		return false, nil
	}

	uname := c.GetCookie(conf.Security.CookieUsername)
	if uname == "" {
		return false, nil
	}

	isSucceed := false
	defer func() {
		if !isSucceed {
			log.Trace("auto-login cookie cleared: %s", uname)
			c.SetCookie(conf.Security.CookieUsername, "", -1, conf.Server.Subpath)
			c.SetCookie(conf.Security.CookieRememberName, "", -1, conf.Server.Subpath)
			c.SetCookie(conf.Security.LoginStatusCookieName, "", -1, conf.Server.Subpath)
		}
	}()

	u, err := database.Users.GetByUsername(c.Req.Context(), uname)
	if err != nil {
		if !database.IsErrUserNotExist(err) {
			return false, fmt.Errorf("get user by name: %v", err)
		}
		return false, nil
	}

	if val, ok := c.GetSuperSecureCookie(u.Rands+u.Password, conf.Security.CookieRememberName); !ok || val != u.Name {
		return false, nil
	}

	isSucceed = true
	_ = c.Session.Set("uid", u.ID)
	_ = c.Session.Set("uname", u.Name)
	c.SetCookie(conf.Session.CSRFCookieName, "", -1, conf.Server.Subpath)
	if conf.Security.EnableLoginStatusCookie {
		c.SetCookie(conf.Security.LoginStatusCookieName, "true", 0, conf.Server.Subpath)
	}
	return true, nil
}

func Login(c *context.Context) {
	c.Title("sign_in")

	// Check auto-login
	isSucceed, err := AutoLogin(c)
	if err != nil {
		c.Error(err, "auto login")
		return
	}

	redirectTo := c.Query("redirect_to")
	if len(redirectTo) > 0 {
		c.SetCookie("redirect_to", redirectTo, 0, conf.Server.Subpath)
	} else {
		redirectTo, _ = url.QueryUnescape(c.GetCookie("redirect_to"))
	}

	if isSucceed {
		if tool.IsSameSiteURLPath(redirectTo) {
			c.Redirect(redirectTo)
		} else {
			c.RedirectSubpath("/")
		}
		c.SetCookie("redirect_to", "", -1, conf.Server.Subpath)
		return
	}

	// Display normal login page
	loginSources, err := database.Handle.LoginSources().List(c.Req.Context(), database.ListLoginSourceOptions{OnlyActivated: true})
	if err != nil {
		c.Error(err, "list activated login sources")
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

func afterLogin(c *context.Context, u *database.User, remember bool) {
	if remember {
		days := 86400 * conf.Security.LoginRememberDays
		c.SetCookie(conf.Security.CookieUsername, u.Name, days, conf.Server.Subpath, "", conf.Security.CookieSecure, true)
		c.SetSuperSecureCookie(u.Rands+u.Password, conf.Security.CookieRememberName, u.Name, days, conf.Server.Subpath, "", conf.Security.CookieSecure, true)
	}

	_ = c.Session.Set("uid", u.ID)
	_ = c.Session.Set("uname", u.Name)
	_ = c.Session.Delete("twoFactorRemember")
	_ = c.Session.Delete("twoFactorUserID")

	// Clear whatever CSRF has right now, force to generate a new one
	c.SetCookie(conf.Session.CSRFCookieName, "", -1, conf.Server.Subpath)
	if conf.Security.EnableLoginStatusCookie {
		c.SetCookie(conf.Security.LoginStatusCookieName, "true", 0, conf.Server.Subpath)
	}

	redirectTo, _ := url.QueryUnescape(c.GetCookie("redirect_to"))
	c.SetCookie("redirect_to", "", -1, conf.Server.Subpath)
	if tool.IsSameSiteURLPath(redirectTo) {
		c.Redirect(redirectTo)
		return
	}

	c.RedirectSubpath("/")
}

func LoginPost(c *context.Context, f form.SignIn) {
	c.Title("sign_in")

	loginSources, err := database.Handle.LoginSources().List(c.Req.Context(), database.ListLoginSourceOptions{OnlyActivated: true})
	if err != nil {
		c.Error(err, "list activated login sources")
		return
	}
	c.Data["LoginSources"] = loginSources

	if c.HasError() {
		c.Success(LOGIN)
		return
	}

	u, err := database.Users.Authenticate(c.Req.Context(), f.UserName, f.Password, f.LoginSource)
	if err != nil {
		switch {
		case auth.IsErrBadCredentials(err):
			c.FormErr("UserName", "Password")
			c.RenderWithErr(c.Tr("form.username_password_incorrect"), LOGIN, &f)
		case database.IsErrLoginSourceMismatch(err):
			c.FormErr("LoginSource")
			c.RenderWithErr(c.Tr("form.auth_source_mismatch"), LOGIN, &f)

		default:
			c.Error(err, "authenticate user")
		}
		for i := range loginSources {
			if loginSources[i].IsDefault {
				c.Data["DefaultLoginSource"] = loginSources[i]
				break
			}
		}
		return
	}

	if !database.Handle.TwoFactors().IsEnabled(c.Req.Context(), u.ID) {
		afterLogin(c, u, f.Remember)
		return
	}

	_ = c.Session.Set("twoFactorRemember", f.Remember)
	_ = c.Session.Set("twoFactorUserID", u.ID)
	c.RedirectSubpath("/user/login/two_factor")
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

	t, err := database.Handle.TwoFactors().GetByUserID(c.Req.Context(), userID)
	if err != nil {
		c.Error(err, "get two factor by user ID")
		return
	}

	passcode := c.Query("passcode")
	valid, err := t.ValidateTOTP(passcode)
	if err != nil {
		c.Error(err, "validate TOTP")
		return
	} else if !valid {
		c.Flash.Error(c.Tr("settings.two_factor_invalid_passcode"))
		c.RedirectSubpath("/user/login/two_factor")
		return
	}

	u, err := database.Users.GetByID(c.Req.Context(), userID)
	if err != nil {
		c.Error(err, "get user by ID")
		return
	}

	// Prevent same passcode from being reused
	if c.Cache.IsExist(userutil.TwoFactorCacheKey(u.ID, passcode)) {
		c.Flash.Error(c.Tr("settings.two_factor_reused_passcode"))
		c.RedirectSubpath("/user/login/two_factor")
		return
	}
	if err = c.Cache.Put(userutil.TwoFactorCacheKey(u.ID, passcode), 1, 60); err != nil {
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

	if err := database.UseRecoveryCode(userID, c.Query("recovery_code")); err != nil {
		if database.IsTwoFactorRecoveryCodeNotFound(err) {
			c.Flash.Error(c.Tr("auth.login_two_factor_invalid_recovery_code"))
			c.RedirectSubpath("/user/login/two_factor_recovery_code")
		} else {
			c.Error(err, "use recovery code")
		}
		return
	}

	u, err := database.Users.GetByID(c.Req.Context(), userID)
	if err != nil {
		c.Error(err, "get user by ID")
		return
	}
	afterLogin(c, u, c.Session.Get("twoFactorRemember").(bool))
}

func SignOut(c *context.Context) {
	_ = c.Session.Flush()
	_ = c.Session.Destory(c.Context)
	c.SetCookie(conf.Security.CookieUsername, "", -1, conf.Server.Subpath)
	c.SetCookie(conf.Security.CookieRememberName, "", -1, conf.Server.Subpath)
	c.SetCookie(conf.Session.CSRFCookieName, "", -1, conf.Server.Subpath)
	c.RedirectSubpath("/")
}

func SignUp(c *context.Context) {
	c.Title("sign_up")

	c.Data["EnableCaptcha"] = conf.Auth.EnableRegistrationCaptcha

	if conf.Auth.DisableRegistration {
		c.Data["DisableRegistration"] = true
		c.Success(SIGNUP)
		return
	}

	c.Success(SIGNUP)
}

func SignUpPost(c *context.Context, cpt *captcha.Captcha, f form.Register) {
	c.Title("sign_up")

	c.Data["EnableCaptcha"] = conf.Auth.EnableRegistrationCaptcha

	if conf.Auth.DisableRegistration {
		c.Status(http.StatusForbidden)
		return
	}

	if c.HasError() {
		c.Success(SIGNUP)
		return
	}

	if conf.Auth.EnableRegistrationCaptcha && !cpt.VerifyReq(c.Req) {
		c.FormErr("Captcha")
		c.RenderWithErr(c.Tr("form.captcha_incorrect"), SIGNUP, &f)
		return
	}

	if f.Password != f.Retype {
		c.FormErr("Password")
		c.RenderWithErr(c.Tr("form.password_not_match"), SIGNUP, &f)
		return
	}

	user, err := database.Users.Create(
		c.Req.Context(),
		f.UserName,
		f.Email,
		database.CreateUserOptions{
			Password:  f.Password,
			Activated: !conf.Auth.RequireEmailConfirmation,
		},
	)
	if err != nil {
		switch {
		case database.IsErrUserAlreadyExist(err):
			c.FormErr("UserName")
			c.RenderWithErr(c.Tr("form.username_been_taken"), SIGNUP, &f)
		case database.IsErrEmailAlreadyUsed(err):
			c.FormErr("Email")
			c.RenderWithErr(c.Tr("form.email_been_used"), SIGNUP, &f)
		case database.IsErrNameNotAllowed(err):
			c.FormErr("UserName")
			c.RenderWithErr(c.Tr("user.form.name_not_allowed", err.(database.ErrNameNotAllowed).Value()), SIGNUP, &f)
		default:
			c.Error(err, "create user")
		}
		return
	}
	log.Trace("Account created: %s", user.Name)

	// FIXME: Count has pretty bad performance implication in large instances, we
	// should have a dedicate method to check whether the "user" table is empty.
	//
	// Auto-set admin for the only user.
	if database.Users.Count(c.Req.Context()) == 1 {
		v := true
		err := database.Users.Update(
			c.Req.Context(),
			user.ID,
			database.UpdateUserOptions{
				IsActivated: &v,
				IsAdmin:     &v,
			},
		)
		if err != nil {
			c.Error(err, "update user")
			return
		}
	}

	// Send confirmation email.
	if conf.Auth.RequireEmailConfirmation && user.ID > 1 {
		email.SendActivateAccountMail(c.Context, database.NewMailerUser(user))
		c.Data["IsSendRegisterMail"] = true
		c.Data["Email"] = user.Email
		c.Data["Hours"] = conf.Auth.ActivateCodeLives / 60
		c.Success(ACTIVATE)

		if err := c.Cache.Put(userutil.MailResendCacheKey(user.ID), 1, 180); err != nil {
			log.Error("Failed to put cache key 'mail resend': %v", err)
		}
		return
	}

	c.RedirectSubpath("/user/login")
}

// parseUserFromCode returns user by username encoded in code.
// It returns nil if code or username is invalid.
func parseUserFromCode(code string) (user *database.User) {
	if len(code) <= tool.TIME_LIMIT_CODE_LENGTH {
		return nil
	}

	// Use tail hex username to query user
	hexStr := code[tool.TIME_LIMIT_CODE_LENGTH:]
	if b, err := hex.DecodeString(hexStr); err == nil {
		if user, err = database.Users.GetByUsername(gocontext.TODO(), string(b)); user != nil {
			return user
		} else if !database.IsErrUserNotExist(err) {
			log.Error("Failed to get user by name %q: %v", string(b), err)
		}
	}

	return nil
}

// verify active code when active account
func verifyUserActiveCode(code string) (user *database.User) {
	minutes := conf.Auth.ActivateCodeLives

	if user = parseUserFromCode(code); user != nil {
		// time limit code
		prefix := code[:tool.TIME_LIMIT_CODE_LENGTH]
		data := com.ToStr(user.ID) + user.Email + user.LowerName + user.Password + user.Rands

		if tool.VerifyTimeLimitCode(data, minutes, prefix) {
			return user
		}
	}
	return nil
}

// verify active code when active account
func verifyActiveEmailCode(code, email string) *database.EmailAddress {
	minutes := conf.Auth.ActivateCodeLives

	if user := parseUserFromCode(code); user != nil {
		// time limit code
		prefix := code[:tool.TIME_LIMIT_CODE_LENGTH]
		data := com.ToStr(user.ID) + email + user.LowerName + user.Password + user.Rands

		if tool.VerifyTimeLimitCode(data, minutes, prefix) {
			emailAddress, err := database.Users.GetEmail(gocontext.TODO(), user.ID, email, false)
			if err == nil {
				return emailAddress
			}
		}
	}
	return nil
}

func Activate(c *context.Context) {
	code := c.Query("code")
	if code == "" {
		c.Data["IsActivatePage"] = true
		if c.User.IsActive {
			c.NotFound()
			return
		}
		// Resend confirmation email.
		if conf.Auth.RequireEmailConfirmation {
			if c.Cache.IsExist(userutil.MailResendCacheKey(c.User.ID)) {
				c.Data["ResendLimited"] = true
			} else {
				c.Data["Hours"] = conf.Auth.ActivateCodeLives / 60
				email.SendActivateAccountMail(c.Context, database.NewMailerUser(c.User))

				if err := c.Cache.Put(userutil.MailResendCacheKey(c.User.ID), 1, 180); err != nil {
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
	if user := verifyUserActiveCode(code); user != nil {
		v := true
		err := database.Users.Update(
			c.Req.Context(),
			user.ID,
			database.UpdateUserOptions{
				GenerateNewRands: true,
				IsActivated:      &v,
			},
		)
		if err != nil {
			c.Error(err, "update user")
			return
		}

		log.Trace("User activated: %s", user.Name)

		_ = c.Session.Set("uid", user.ID)
		_ = c.Session.Set("uname", user.Name)
		c.RedirectSubpath("/")
		return
	}

	c.Data["IsActivateFailed"] = true
	c.Success(ACTIVATE)
}

func ActivateEmail(c *context.Context) {
	code := c.Query("code")
	emailAddr := c.Query("email")

	// Verify code.
	if email := verifyActiveEmailCode(code, emailAddr); email != nil {
		err := database.Users.MarkEmailActivated(c.Req.Context(), email.UserID, email.Email)
		if err != nil {
			c.Error(err, "activate email")
			return
		}

		log.Trace("Email activated: %s", email.Email)
		c.Flash.Success(c.Tr("settings.add_email_success"))
	}

	c.RedirectSubpath("/user/settings/email")
}

func ForgotPasswd(c *context.Context) {
	c.Title("auth.forgot_password")

	if !conf.Email.Enabled {
		c.Data["IsResetDisable"] = true
		c.Success(FORGOT_PASSWORD)
		return
	}

	c.Data["IsResetRequest"] = true
	c.Success(FORGOT_PASSWORD)
}

func ForgotPasswdPost(c *context.Context) {
	c.Title("auth.forgot_password")

	if !conf.Email.Enabled {
		c.Status(403)
		return
	}
	c.Data["IsResetRequest"] = true

	emailAddr := c.Query("email")
	c.Data["Email"] = emailAddr

	u, err := database.Users.GetByEmail(c.Req.Context(), emailAddr)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			c.Data["Hours"] = conf.Auth.ActivateCodeLives / 60
			c.Data["IsResetSent"] = true
			c.Success(FORGOT_PASSWORD)
			return
		}

		c.Error(err, "get user by email")
		return
	}

	if !u.IsLocal() {
		c.FormErr("Email")
		c.RenderWithErr(c.Tr("auth.non_local_account"), FORGOT_PASSWORD, nil)
		return
	}

	if c.Cache.IsExist(userutil.MailResendCacheKey(u.ID)) {
		c.Data["ResendLimited"] = true
		c.Success(FORGOT_PASSWORD)
		return
	}

	email.SendResetPasswordMail(c.Context, database.NewMailerUser(u))
	if err = c.Cache.Put(userutil.MailResendCacheKey(u.ID), 1, 180); err != nil {
		log.Error("Failed to put cache key 'mail resend': %v", err)
	}

	c.Data["Hours"] = conf.Auth.ActivateCodeLives / 60
	c.Data["IsResetSent"] = true
	c.Success(FORGOT_PASSWORD)
}

func ResetPasswd(c *context.Context) {
	c.Title("auth.reset_password")

	code := c.Query("code")
	if code == "" {
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
	if code == "" {
		c.NotFound()
		return
	}
	c.Data["Code"] = code

	if u := verifyUserActiveCode(code); u != nil {
		// Validate password length.
		password := c.Query("password")
		if len(password) < 6 {
			c.Data["IsResetForm"] = true
			c.Data["Err_Password"] = true
			c.RenderWithErr(c.Tr("auth.password_too_short"), RESET_PASSWORD, nil)
			return
		}

		err := database.Users.Update(c.Req.Context(), u.ID, database.UpdateUserOptions{Password: &password})
		if err != nil {
			c.Error(err, "update user")
			return
		}

		log.Trace("User password reset: %s", u.Name)
		c.RedirectSubpath("/user/login")
		return
	}

	c.Data["IsResetFailed"] = true
	c.Success(RESET_PASSWORD)
}
