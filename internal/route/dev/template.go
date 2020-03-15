// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package dev

import (
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
)

func TemplatePreview(c *context.Context) {
	c.Data["User"] = db.User{Name: "Unknown"}
	c.Data["AppName"] = conf.App.BrandName
	c.Data["AppVersion"] = conf.App.Version
	c.Data["AppURL"] = conf.Server.ExternalURL
	c.Data["Code"] = "2014031910370000009fff6782aadb2162b4a997acb69d4400888e0b9274657374"
	c.Data["ActiveCodeLives"] = conf.Auth.ActivateCodeLives / 60
	c.Data["ResetPwdCodeLives"] = conf.Auth.ResetPasswordCodeLives / 60
	c.Data["CurDbValue"] = ""

	c.Success( (c.Params("*")))
}
