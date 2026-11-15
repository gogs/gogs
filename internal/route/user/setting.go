package user

import (
	gocontext "context"
	"encoding/hex"

	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/tool"
	"gogs.io/gogs/internal/userx"
)

func parseUserFromCode(ctx gocontext.Context, code string) (user *database.User) {
	if len(code) <= tool.TimeLimitCodeLength {
		return nil
	}

	hexStr := code[tool.TimeLimitCodeLength:]
	if b, err := hex.DecodeString(hexStr); err == nil {
		if user, err = database.Handle.Users().GetByUsername(ctx, string(b)); user != nil {
			return user
		} else if !database.IsErrUserNotExist(err) {
			log.Error("parseUserFromCode: get user by name %q: %v", string(b), err)
		}
	}
	return nil
}

func verifyActiveEmailCode(ctx gocontext.Context, code, email string) *database.EmailAddress {
	if user := parseUserFromCode(ctx, code); user != nil {
		if userx.VerifyActivateCode(code, user.ID, email, user.LowerName, user.Password, user.Rands) {
			emailAddress, err := database.Handle.Users().GetEmail(ctx, user.ID, email, false)
			if err == nil {
				return emailAddress
			}
		}
	}
	return nil
}

func ActivateEmail(c *context.Context) {
	code := c.Query("code")
	emailAddr := c.Query("email")

	if email := verifyActiveEmailCode(c.Req.Context(), code, emailAddr); email != nil {
		err := database.Handle.Users().MarkEmailActivated(c.Req.Context(), email.UserID, email.Email)
		if err != nil {
			c.Error(err, "activate email")
			return
		}

		log.Trace("Email activated: %s", email.Email)
		// The settings page confirms the activation.
		c.RedirectSubpath("/user/settings/email?activated=1")
		return
	}

	c.RedirectSubpath("/user/settings/email")
}
