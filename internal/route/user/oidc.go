// Copyright 2024 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"fmt"
	"net/url"

	"github.com/unknwon/com"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/auth/oidc"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/tool"
)

// OIDCLogin starts the OIDC login flow
func OIDCLogin(c *context.Context) {
	loginSourceID := com.StrTo(c.Params(":id")).MustInt64()
	if loginSourceID == 0 {
		c.NotFound()
		return
	}

	loginSource, err := database.Handle.LoginSources().GetByID(c.Req.Context(), loginSourceID)
	if err != nil {
		c.Error(err, "get login source by ID")
		return
	}

	if !loginSource.IsOIDC() {
		c.NotFound()
		return
	}

	provider := loginSource.Provider.(*oidc.Provider)
	_, oauth2Config, err := provider.GetOAuth2Config(c.Req.Context())
	if err != nil {
		c.Error(err, "get oauth2 config")
		return
	}

	// Build redirect URL - use configured URL if available, otherwise build it
	oidcConfig := loginSource.OIDC()
	callbackURL := oidcConfig.RedirectURL
	if callbackURL == "" {
		callbackURL = fmt.Sprintf("%suser/oauth2/%d/callback", conf.Server.ExternalURL, loginSourceID)
	}
	oauth2Config.RedirectURL = callbackURL

	// Store state for security
	state := tool.ShortSHA1(com.ToStr(loginSourceID))
	_ = c.Session.Set("oidc_state", state)
	_ = c.Session.Set("oidc_login_source_id", loginSourceID)

	authURL := oauth2Config.AuthCodeURL(state)
	c.Resp.Header().Set("Location", authURL)
	c.Resp.WriteHeader(302)
}

// OIDCCallback handles the OIDC callback
func OIDCCallback(c *context.Context) {
	// Verify state
	state := c.Query("state")
	sessionState := c.Session.Get("oidc_state")
	if sessionState == nil || sessionState.(string) != state {
		c.RenderWithErr("Invalid state parameter", tmplUserAuthLogin, nil)
		return
	}

	loginSourceID := c.Session.Get("oidc_login_source_id")
	if loginSourceID == nil {
		c.RenderWithErr("No login source in session", tmplUserAuthLogin, nil)
		return
	}

	// Clear session state
	_ = c.Session.Delete("oidc_state")
	_ = c.Session.Delete("oidc_login_source_id")

	// Get login source
	loginSource, err := database.Handle.LoginSources().GetByID(c.Req.Context(), loginSourceID.(int64))
	if err != nil {
		c.Error(err, "get login source by ID")
		return
	}

	// Get authorization code
	code := c.Query("code")
	if code == "" {
		c.RenderWithErr("No authorization code received", tmplUserAuthLogin, nil)
		return
	}

	// Authenticate with OIDC provider
	provider := loginSource.Provider.(*oidc.Provider)
	extAccount, err := provider.AuthenticateUser(c.Req.Context(), code)
	if err != nil {
		log.Error("Failed to authenticate with OIDC provider: %v", err)
		c.RenderWithErr("Failed to authenticate with OIDC provider", tmplUserAuthLogin, nil)
		return
	}

	// Try to find existing user
	user, err := database.Handle.Users().GetByEmail(c.Req.Context(), extAccount.Email)
	if err != nil && !database.IsErrUserNotExist(err) {
		c.Error(err, "get user by email")
		return
	}

	// If user doesn't exist, create one if auto-registration is enabled
	if user == nil {
		oidcConfig := loginSource.OIDC()
		if !oidcConfig.AutoRegister {
			c.RenderWithErr("User not found and auto-registration is disabled", tmplUserAuthLogin, nil)
			return
		}

		// Create new user
		user, err = database.Handle.Users().Create(
			c.Req.Context(),
			extAccount.Login,
			extAccount.Email,
			database.CreateUserOptions{
				FullName:  extAccount.FullName,
				Activated: true, // OIDC users are pre-verified
			},
		)
		if err != nil {
			c.Error(err, "create user from OIDC")
			return
		}
	}

	// Log in the user
	afterLogin(c, user, false)

	// Handle redirect
	redirectTo, _ := url.QueryUnescape(c.GetCookie("redirect_to"))
	if tool.IsSameSiteURLPath(redirectTo) {
		c.Redirect(redirectTo)
	} else {
		c.RedirectSubpath("/")
	}
	c.SetCookie("redirect_to", "", -1, conf.Server.Subpath)
}