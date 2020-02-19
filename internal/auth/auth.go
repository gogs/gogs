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

	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/db/errors"
	"gogs.io/gogs/internal/setting"
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
			t, err := db.GetAccessTokenBySHA(tokenSHA)
			if err != nil {
				if !db.IsErrAccessTokenNotExist(err) && !db.IsErrAccessTokenEmpty(err) {
					log.Error("GetAccessTokenBySHA: %v", err)
				}
				return 0, false
			}
			t.Updated = time.Now()
			if err = db.UpdateAccessToken(t); err != nil {
				log.Error("UpdateAccessToken: %v", err)
			}
			return t.UID, true
		}
	}

	uid := sess.Get("uid")
	if uid == nil {
		return 0, false
	}
	if id, ok := uid.(int64); ok {
		if _, err := db.GetUserByID(id); err != nil {
			if !errors.IsUserNotExist(err) {
				log.Error("GetUserByID: %v", err)
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
		if setting.Service.EnableReverseProxyAuth {
			webAuthUser := ctx.Req.Header.Get(setting.ReverseProxyAuthUser)
			if len(webAuthUser) > 0 {
				u, err := db.GetUserByName(webAuthUser)
				if err != nil {
					if !errors.IsUserNotExist(err) {
						log.Error("GetUserByName: %v", err)
						return nil, false, false
					}

					// Check if enabled auto-registration.
					if setting.Service.EnableReverseProxyAutoRegister {
						u := &db.User{
							Name:     webAuthUser,
							Email:    gouuid.NewV4().String() + "@localhost",
							Passwd:   webAuthUser,
							IsActive: true,
						}
						if err = db.CreateUser(u); err != nil {
							// FIXME: should I create a system notice?
							log.Error("CreateUser: %v", err)
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

				u, err := db.UserLogin(uname, passwd, -1)
				if err != nil {
					if !errors.IsUserNotExist(err) {
						log.Error("UserLogin: %v", err)
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
