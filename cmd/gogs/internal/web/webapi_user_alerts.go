package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"gopkg.in/macaron.v1"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/email"
)

// alertUser emails the user about a change to the security of their account,
// with the browser and the IP address it came from. The change already
// happened, so failing to compose the email does not fail the request.
func alertUser(mc *macaron.Context, u *database.User, to string, alert email.SecurityAlert, name string) {
	activity := email.Activity{
		UserAgent: mc.Req.UserAgent(),
		IP:        strings.Trim(mc.RemoteAddr(), "[]"),
		Time:      time.Now(),
	}
	if err := email.SendSecurityAlertMail(mc, database.NewMailerUser(u), to, alert, activity, name); err != nil {
		log.Error("Failed to alert user %d: %v", u.ID, err)
	}
}

// knownBrowserCookie marks a browser that a user signed in from, so signing in
// from it again does not alert them. It holds the mark of one user, the one
// who signed in last.
const knownBrowserCookie = "gogs_known_browser"

func knownBrowserMark(userID int64) string {
	mac := hmac.New(sha256.New, []byte(conf.Security.SecretKey))
	mac.Write([]byte("known browser " + strconv.FormatInt(userID, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

func isKnownBrowser(mc *macaron.Context, userID int64) bool {
	return hmac.Equal([]byte(mc.GetCookie(knownBrowserCookie)), []byte(knownBrowserMark(userID)))
}

func rememberBrowser(mc *macaron.Context, userID int64) {
	const oneYear = 365 * 24 * 60 * 60
	mc.SetCookie(knownBrowserCookie, knownBrowserMark(userID), oneYear, conf.Server.Subpath, "", conf.Security.CookieSecure, true)
}
