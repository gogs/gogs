package user

import (
	gocontext "context"
	"encoding/hex"
	"strconv"

	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/email"
	"gogs.io/gogs/internal/tool"
	"gogs.io/gogs/internal/userx"
)

const TmplUserAuthActivate = "user/auth/activate"

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
