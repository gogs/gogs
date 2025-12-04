// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package context

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-macaron/csrf"
	"github.com/go-macaron/session"
	"github.com/pkg/errors"
	gouuid "github.com/satori/go.uuid"
	"gopkg.in/macaron.v1"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/tool"
)

type ToggleOptions struct {
	SignInRequired  bool
	SignOutRequired bool
	AdminRequired   bool
	DisableCSRF     bool
}

func Toggle(options *ToggleOptions) macaron.Handler {
	return func(c *Context) {
		// Cannot view any page before installation.
		if !conf.Security.InstallLock {
			c.RedirectSubpath("/install")
			return
		}

		// Check prohibit login users.
		if c.IsLogged && c.User.ProhibitLogin {
			c.Data["Title"] = c.Tr("auth.prohibit_login")
			c.Success("user/auth/prohibit_login")
			return
		}

		// Check non-logged users landing page.
		if !c.IsLogged && c.Req.RequestURI == "/" && conf.Server.LandingURL != "/" {
			c.RedirectSubpath(conf.Server.LandingURL)
			return
		}

		// Redirect to dashboard if user tries to visit any non-login page.
		if options.SignOutRequired && c.IsLogged && c.Req.RequestURI != "/" {
			c.RedirectSubpath("/")
			return
		}

		if !options.SignOutRequired && !options.DisableCSRF && c.Req.Method == "POST" && !isAPIPath(c.Req.URL.Path) {
			csrf.Validate(c.Context, c.csrf)
			if c.Written() {
				return
			}
		}

		if options.SignInRequired {
			if !c.IsLogged {
				// Restrict API calls with error message.
				if isAPIPath(c.Req.URL.Path) {
					c.JSON(http.StatusForbidden, map[string]string{
						"message": "Only authenticated user is allowed to call APIs.",
					})
					return
				}

				c.SetCookie("redirect_to", url.QueryEscape(conf.Server.Subpath+c.Req.RequestURI), 0, conf.Server.Subpath)
				c.RedirectSubpath("/user/login")
				return
			} else if !c.User.IsActive && conf.Auth.RequireEmailConfirmation {
				c.Title("auth.active_your_account")
				c.Success("user/auth/activate")
				return
			}
		}

		// Redirect to log in page if auto-signin info is provided and has not signed in.
		if !options.SignOutRequired && !c.IsLogged && !isAPIPath(c.Req.URL.Path) &&
			len(c.GetCookie(conf.Security.CookieUsername)) > 0 {
			c.SetCookie("redirect_to", url.QueryEscape(conf.Server.Subpath+c.Req.RequestURI), 0, conf.Server.Subpath)
			c.RedirectSubpath("/user/login")
			return
		}

		if options.AdminRequired {
			if !c.User.IsAdmin {
				c.Status(http.StatusForbidden)
				return
			}
			c.PageIs("Admin")
		}
	}
}

func isAPIPath(url string) bool {
	return strings.HasPrefix(url, "/api/")
}

type AuthStore interface {
	// GetAccessTokenBySHA1 returns the access token with given SHA1. It returns
	// database.ErrAccessTokenNotExist when not found.
	GetAccessTokenBySHA1(ctx context.Context, sha1 string) (*database.AccessToken, error)
	// TouchAccessTokenByID updates the updated time of the given access token to
	// the current time.
	TouchAccessTokenByID(ctx context.Context, id int64) error

	// GetUserByID returns the user with given ID. It returns
	// database.ErrUserNotExist when not found.
	GetUserByID(ctx context.Context, id int64) (*database.User, error)
	// GetUserByUsername returns the user with given username. It returns
	// database.ErrUserNotExist when not found.
	GetUserByUsername(ctx context.Context, username string) (*database.User, error)
	// CreateUser creates a new user and persists to database. It returns
	// database.ErrNameNotAllowed if the given name or pattern of the name is not
	// allowed as a username, or database.ErrUserAlreadyExist when a user with same
	// name already exists, or database.ErrEmailAlreadyUsed if the email has been
	// verified by another user.
	CreateUser(ctx context.Context, username, email string, opts database.CreateUserOptions) (*database.User, error)
	// AuthenticateUser validates username and password via given login source ID.
	// It returns database.ErrUserNotExist when the user was not found.
	//
	// When the "loginSourceID" is negative, it aborts the process and returns
	// database.ErrUserNotExist if the user was not found in the database.
	//
	// When the "loginSourceID" is non-negative, it returns
	// database.ErrLoginSourceMismatch if the user has different login source ID
	// than the "loginSourceID".
	//
	// When the "loginSourceID" is positive, it tries to authenticate via given
	// login source and creates a new user when not yet exists in the database.
	AuthenticateUser(ctx context.Context, login, password string, loginSourceID int64) (*database.User, error)
}

// authenticatedUserID returns the ID of the authenticated user, along with a bool value
// which indicates whether the user uses token authentication.
func authenticatedUserID(store AuthStore, c *macaron.Context, sess session.Store) (_ int64, isTokenAuth bool) {
	if !database.HasEngine {
		return 0, false
	}

	// Check access token.
	if isAPIPath(c.Req.URL.Path) {
		tokenSHA := c.Query("token")
		if len(tokenSHA) <= 0 {
			tokenSHA = c.Query("access_token")
		}
		if tokenSHA == "" {
			// Well, check with header again.
			auHead := c.Req.Header.Get("Authorization")
			if len(auHead) > 0 {
				auths := strings.Fields(auHead)
				if len(auths) == 2 && auths[0] == "token" {
					tokenSHA = auths[1]
				}
			}
		}

		// Let's see if token is valid.
		if len(tokenSHA) > 0 {
			t, err := store.GetAccessTokenBySHA1(c.Req.Context(), tokenSHA)
			if err != nil {
				if !database.IsErrAccessTokenNotExist(err) {
					log.Error("GetAccessTokenBySHA: %v", err)
				}
				return 0, false
			}
			if err = store.TouchAccessTokenByID(c.Req.Context(), t.ID); err != nil {
				log.Error("Failed to touch access token: %v", err)
			}
			return t.UserID, true
		}
	}

	uid := sess.Get("uid")
	if uid == nil {
		return 0, false
	}
	if id, ok := uid.(int64); ok {
		_, err := store.GetUserByID(c.Req.Context(), id)
		if err != nil {
			if !database.IsErrUserNotExist(err) {
				log.Error("Failed to get user by ID: %v", err)
			}
			return 0, false
		}
		return id, false
	}
	return 0, false
}

// authenticatedUser returns the user object of the authenticated user, along with two bool values
// which indicate whether the user uses HTTP Basic Authentication or token authentication respectively.
func authenticatedUser(store AuthStore, ctx *macaron.Context, sess session.Store) (_ *database.User, isBasicAuth, isTokenAuth bool) {
	if !database.HasEngine {
		return nil, false, false
	}

	uid, isTokenAuth := authenticatedUserID(store, ctx, sess)

	if uid <= 0 {
		if conf.Auth.EnableReverseProxyAuthentication {
			webAuthUser := ctx.Req.Header.Get(conf.Auth.ReverseProxyAuthenticationHeader)
			if len(webAuthUser) > 0 {
				user, err := store.GetUserByUsername(ctx.Req.Context(), webAuthUser)
				if err != nil {
					if !database.IsErrUserNotExist(err) {
						log.Error("Failed to get user by name: %v", err)
						return nil, false, false
					}

					// Check if enabled auto-registration.
					if conf.Auth.EnableReverseProxyAutoRegistration {
						user, err = store.CreateUser(
							ctx.Req.Context(),
							webAuthUser,
							gouuid.NewV4().String()+"@localhost",
							database.CreateUserOptions{
								Activated: true,
							},
						)
						if err != nil {
							log.Error("Failed to create user %q: %v", webAuthUser, err)
							return nil, false, false
						}
					}
				}
				return user, false, false
			}
		}

		// Check with basic auth.
		baHead := ctx.Req.Header.Get("Authorization")
		if len(baHead) > 0 {
			auths := strings.Fields(baHead)
			if len(auths) == 2 && auths[0] == "Basic" {
				uname, passwd, _ := tool.BasicAuthDecode(auths[1])

				u, err := store.AuthenticateUser(ctx.Req.Context(), uname, passwd, -1)
				if err != nil {
					if !auth.IsErrBadCredentials(err) {
						log.Error("Failed to authenticate user: %v", err)
					}
					return nil, false, false
				}

				return u, true, false
			}
		}
		return nil, false, false
	}

	u, err := store.GetUserByID(ctx.Req.Context(), uid)
	if err != nil {
		log.Error("GetUserByID: %v", err)
		return nil, false, false
	}
	return u, false, isTokenAuth
}

// AuthenticateByToken attempts to authenticate a user by the given access
// token. It returns database.ErrAccessTokenNotExist when the access token does not
// exist.
func AuthenticateByToken(store AuthStore, ctx context.Context, token string) (*database.User, error) {
	t, err := store.GetAccessTokenBySHA1(ctx, token)
	if err != nil {
		return nil, errors.Wrap(err, "get access token by SHA1")
	}
	if err = store.TouchAccessTokenByID(ctx, t.ID); err != nil {
		// NOTE: There is no need to fail the auth flow if we can't touch the token.
		log.Error("Failed to touch access token [id: %d]: %v", t.ID, err)
	}

	user, err := store.GetUserByID(ctx, t.UserID)
	if err != nil {
		return nil, errors.Wrapf(err, "get user by ID [user_id: %d]", t.UserID)
	}
	return user, nil
}
