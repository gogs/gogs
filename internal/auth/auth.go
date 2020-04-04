// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"strings"
	"time"

	"github.com/go-macaron/session"
	gouuid "github.com/satori/go.uuid"
	"gopkg.in/macaron.v1"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/tool"
)

func IsAPIPath(url string) bool {
	return strings.HasPrefix(url, "/api/")
}

// SignedInID returns the id of signed in user, along with one bool value which indicates whether user uses token
// authentication.
func SignedInID(c *macaron.Context, sess session.Store) (_ int64, isTokenAuth bool) {
	if !db.HasEngine {
		return 0, false
	}

	// Check access token.
	if IsAPIPath(c.Req.URL.Path) {
		tokenSHA := c.Query("token")
		if len(tokenSHA) <= 0 {
			tokenSHA = c.Query("access_token")
		}
		if len(tokenSHA) == 0 {
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
			t, err := db.AccessTokens.GetBySHA(tokenSHA)
			if err != nil {
				if !db.IsErrAccessTokenNotExist(err) {
					log.Error("GetAccessTokenBySHA: %v", err)
				}
				return 0, false
			}
			t.Updated = time.Now()
			if err = db.AccessTokens.Save(t); err != nil {
				log.Error("UpdateAccessToken: %v", err)
			}
			return t.UserID, true
		}
	}

	uid := sess.Get("uid")
	if uid == nil {
		return 0, false
	}
	if id, ok := uid.(int64); ok {
		if _, err := db.GetUserByID(id); err != nil {
			if !db.IsErrUserNotExist(err) {
				log.Error("Failed to get user by ID: %v", err)
			}
			return 0, false
		}
		return id, false
	}
	return 0, false
}

// SignedInUser returns the user object of signed in user, along with two bool values,
// which indicate whether user uses HTTP Basic Authentication or token authentication respectively.
func SignedInUser(ctx *macaron.Context, sess session.Store) (_ *db.User, isBasicAuth bool, isTokenAuth bool) {
	if !db.HasEngine {
		return nil, false, false
	}

	uid, isTokenAuth := SignedInID(ctx, sess)

	if uid <= 0 {
		if conf.Auth.EnableReverseProxyAuthentication {
			webAuthUser := ctx.Req.Header.Get(conf.Auth.ReverseProxyAuthenticationHeader)
			if len(webAuthUser) > 0 {
				u, err := db.GetUserByName(webAuthUser)
				if err != nil {
					if !db.IsErrUserNotExist(err) {
						log.Error("Failed to get user by name: %v", err)
						return nil, false, false
					}

					// Check if enabled auto-registration.
					if conf.Auth.EnableReverseProxyAutoRegistration {
						u := &db.User{
							Name:     webAuthUser,
							Email:    gouuid.NewV4().String() + "@localhost",
							Passwd:   webAuthUser,
							IsActive: true,
						}
						if err = db.CreateUser(u); err != nil {
							// FIXME: should I create a system notice?
							log.Error("Failed to create user: %v", err)
							return nil, false, false
						} else {
							return u, false, false
						}
					}
				}
				return u, false, false
			}
		}

		// Check with basic auth.
		baHead := ctx.Req.Header.Get("Authorization")
		if len(baHead) > 0 {
			auths := strings.Fields(baHead)
			if len(auths) == 2 && auths[0] == "Basic" {
				uname, passwd, _ := tool.BasicAuthDecode(auths[1])

				u, err := db.Users.Authenticate(uname, passwd, -1)
				if err != nil {
					if !db.IsErrUserNotExist(err) {
						log.Error("Failed to authenticate user: %v", err)
					}
					return nil, false, false
				}

				return u, true, false
			}
		}
		return nil, false, false
	}

	u, err := db.GetUserByID(uid)
	if err != nil {
		log.Error("GetUserByID: %v", err)
		return nil, false, false
	}
	return u, false, isTokenAuth
}
