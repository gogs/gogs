package user

import (
	gocontext "context"
	"encoding/hex"
	"net/http"
	"strconv"

	"github.com/go-macaron/captcha"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/email"
	"gogs.io/gogs/internal/form"
	"gogs.io/gogs/internal/tool"
	"gogs.io/gogs/internal/userx"
)

const (
	tmplUserAuthSignup         = "user/auth/signup"
	TmplUserAuthActivate       = "user/auth/activate"
	tmplUserAuthForgotPassword = "user/auth/forgot_passwd"
	tmplUserAuthResetPassword  = "user/auth/reset_passwd"
)

func SignOut(c *context.Context) {
	_ = c.Session.Flush()
	_ = c.Session.Destory(c.Context)
	c.SetCookie(conf.Session.CSRFCookieName, "", -1, conf.Server.Subpath)
	if conf.Auth.CustomLogoutURL != "" {
		c.Redirect(conf.Auth.CustomLogoutURL)
		return
	}
	c.RedirectSubpath("/")
}

func SignUp(c *context.Context) {
	c.Title("sign_up")

	c.Data["EnableCaptcha"] = conf.Auth.EnableRegistrationCaptcha

	if conf.Auth.DisableRegistration {
		c.Data["DisableRegistration"] = true
		c.Success(tmplUserAuthSignup)
		return
	}

	c.Success(tmplUserAuthSignup)
}

func SignUpPost(c *context.Context, cpt *captcha.Captcha, f form.Register) {
	c.Title("sign_up")

	c.Data["EnableCaptcha"] = conf.Auth.EnableRegistrationCaptcha

	if conf.Auth.DisableRegistration {
		c.Status(http.StatusForbidden)
		return
	}

	if c.HasError() {
		c.HTML(http.StatusBadRequest, tmplUserAuthSignup)
		return
	}

	if conf.Auth.EnableRegistrationCaptcha && !cpt.VerifyReq(c.Req) {
		c.FormErr("Captcha")
		c.RenderWithErr(c.Tr("form.captcha_incorrect"), http.StatusUnauthorized, tmplUserAuthSignup, &f)
		return
	}

	if f.Password != f.Retype {
		c.FormErr("Password")
		c.RenderWithErr(c.Tr("form.password_not_match"), http.StatusBadRequest, tmplUserAuthSignup, &f)
		return
	}

	user, err := database.Handle.Users().Create(
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
			c.RenderWithErr(c.Tr("form.username_been_taken"), http.StatusUnprocessableEntity, tmplUserAuthSignup, &f)
		case database.IsErrEmailAlreadyUsed(err):
			c.FormErr("Email")
			c.RenderWithErr(c.Tr("form.email_been_used"), http.StatusUnprocessableEntity, tmplUserAuthSignup, &f)
		case database.IsErrNameNotAllowed(err):
			c.FormErr("UserName")
			c.RenderWithErr(c.Tr("user.form.name_not_allowed", err.(database.ErrNameNotAllowed).Value()), http.StatusBadRequest, tmplUserAuthSignup, &f)
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
	if database.Handle.Users().Count(c.Req.Context()) == 1 {
		v := true
		err := database.Handle.Users().Update(
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
		if err := email.SendActivateAccountMail(c.Context, database.NewMailerUser(user)); err != nil {
			log.Error("Failed to send activate account mail: %v", err)
		}
		c.Data["IsSendRegisterMail"] = true
		c.Data["Email"] = user.Email
		c.Data["Hours"] = conf.Auth.ActivateCodeLives / 60
		c.Success(TmplUserAuthActivate)

		if err := c.Cache.Put(userx.MailResendCacheKey(user.ID), 1, 180); err != nil {
			log.Error("Failed to put cache key 'mail resend': %v", err)
		}
		return
	}

	c.RedirectSubpath("/user/sign-in")
}

// parseUserFromCode returns user by username encoded in code.
// It returns nil if code or username is invalid.
func parseUserFromCode(code string) (user *database.User) {
	if len(code) <= tool.TimeLimitCodeLength {
		return nil
	}

	// Use tail hex username to query user
	hexStr := code[tool.TimeLimitCodeLength:]
	if b, err := hex.DecodeString(hexStr); err == nil {
		if user, err = database.Handle.Users().GetByUsername(gocontext.TODO(), string(b)); user != nil {
			return user
		} else if !database.IsErrUserNotExist(err) {
			log.Error("Failed to get user by name %q: %v", string(b), err)
		}
	}

	return nil
}

// VerifyUserActiveCode verifies an account activation or password reset code.
func VerifyUserActiveCode(code string) (user *database.User) {
	minutes := conf.Auth.ActivateCodeLives

	if user = parseUserFromCode(code); user != nil {
		// time limit code
		prefix := code[:tool.TimeLimitCodeLength]
		data := strconv.FormatInt(user.ID, 10) + user.Email + user.LowerName + user.Password + user.Rands

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
		prefix := code[:tool.TimeLimitCodeLength]
		data := strconv.FormatInt(user.ID, 10) + email + user.LowerName + user.Password + user.Rands

		if tool.VerifyTimeLimitCode(data, minutes, prefix) {
			emailAddress, err := database.Handle.Users().GetEmail(gocontext.TODO(), user.ID, email, false)
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
			if c.Cache.IsExist(userx.MailResendCacheKey(c.User.ID)) {
				c.Data["ResendLimited"] = true
			} else {
				c.Data["Hours"] = conf.Auth.ActivateCodeLives / 60
				if err := email.SendActivateAccountMail(c.Context, database.NewMailerUser(c.User)); err != nil {
					log.Error("Failed to send activate account mail: %v", err)
				}

				if err := c.Cache.Put(userx.MailResendCacheKey(c.User.ID), 1, 180); err != nil {
					log.Error("Failed to put cache key 'mail resend': %v", err)
				}
			}
		} else {
			c.Data["ServiceNotEnabled"] = true
		}
		c.Success(TmplUserAuthActivate)
		return
	}

	// Verify code.
	if user := VerifyUserActiveCode(code); user != nil {
		v := true
		err := database.Handle.Users().Update(
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
	c.Success(TmplUserAuthActivate)
}

func ActivateEmail(c *context.Context) {
	code := c.Query("code")
	emailAddr := c.Query("email")

	// Verify code.
	if email := verifyActiveEmailCode(code, emailAddr); email != nil {
		err := database.Handle.Users().MarkEmailActivated(c.Req.Context(), email.UserID, email.Email)
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
		c.Success(tmplUserAuthForgotPassword)
		return
	}

	c.Data["IsResetRequest"] = true
	c.Success(tmplUserAuthForgotPassword)
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

	u, err := database.Handle.Users().GetByEmail(c.Req.Context(), emailAddr)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			c.Data["Hours"] = conf.Auth.ActivateCodeLives / 60
			c.Data["IsResetSent"] = true
			c.Success(tmplUserAuthForgotPassword)
			return
		}

		c.Error(err, "get user by email")
		return
	}

	if !u.IsLocal() {
		c.FormErr("Email")
		c.RenderWithErr(c.Tr("auth.non_local_account"), http.StatusForbidden, tmplUserAuthForgotPassword, nil)
		return
	}

	if c.Cache.IsExist(userx.MailResendCacheKey(u.ID)) {
		c.Data["ResendLimited"] = true
		c.Success(tmplUserAuthForgotPassword)
		return
	}

	if err = email.SendResetPasswordMail(c.Context, database.NewMailerUser(u)); err != nil {
		log.Error("Failed to send reset password mail: %v", err)
	}
	if err = c.Cache.Put(userx.MailResendCacheKey(u.ID), 1, 180); err != nil {
		log.Error("Failed to put cache key 'mail resend': %v", err)
	}

	c.Data["Hours"] = conf.Auth.ActivateCodeLives / 60
	c.Data["IsResetSent"] = true
	c.Success(tmplUserAuthForgotPassword)
}

func ResetPasswd(c *context.Context) {
	code := c.Query("code")
	if code == "" {
		c.NotFound()
		return
	}
	c.ServeWeb()
}

func ResetPasswdPost(c *context.Context) {
	c.Title("auth.reset_password")

	code := c.Query("code")
	if code == "" {
		c.NotFound()
		return
	}
	c.Data["Code"] = code

	if u := VerifyUserActiveCode(code); u != nil {
		// Validate password length.
		password := c.Query("password")
		if len(password) < 6 {
			c.Data["IsResetForm"] = true
			c.Data["Err_Password"] = true
			c.RenderWithErr(c.Tr("auth.password_too_short"), http.StatusBadRequest, tmplUserAuthResetPassword, nil)
			return
		}

		err := database.Handle.Users().Update(c.Req.Context(), u.ID, database.UpdateUserOptions{Password: &password})
		if err != nil {
			c.Error(err, "update user")
			return
		}

		log.Trace("User password reset: %s", u.Name)
		c.RedirectSubpath("/user/sign-in")
		return
	}

	c.Data["IsResetFailed"] = true
	c.Success(tmplUserAuthResetPassword)
}
