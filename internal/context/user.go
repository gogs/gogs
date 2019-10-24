// Copyright 2018 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package context

import (
	"gopkg.in/macaron.v1"

	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/db/errors"
)

// ParamsUser is the wrapper type of the target user defined by URL parameter, namely ':username'.
type ParamsUser struct {
	*db.User
}

// InjectParamsUser returns a handler that retrieves target user based on URL parameter ':username',
// and injects it as *ParamsUser.
func InjectParamsUser() macaron.Handler {
	return func(c *Context) {
		user, err := db.GetUserByName(c.Params(":username"))
		if err != nil {
			c.NotFoundOrServerError("GetUserByName", errors.IsUserNotExist, err)
			return
		}
		c.Map(&ParamsUser{user})
	}
}
