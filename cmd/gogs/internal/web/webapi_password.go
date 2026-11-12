package web

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/go-macaron/i18n"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/userx"
)

// checkNewPassword checks a password being set against the password policy of
// the site, and returns the message for the password field when it fails.
func checkNewPassword(ctx context.Context, l i18n.Locale, password string) *string {
	var msg string
	switch err := userx.CheckPasswordPolicy(ctx, password); {
	case errors.Is(err, userx.ErrPasswordTooShort):
		msg = l.Tr("form.password_too_short", conf.Auth.MinPasswordLength)
	case errors.Is(err, userx.ErrPasswordLeaked):
		msg = l.Tr("form.password_leaked")
	default:
		return nil
	}
	return &msg
}
