// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package dev

import (
	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/pkg/context"
	"github.com/gogs/gogs/pkg/setting"
)

func TemplatePreview(c *context.Context) {
	c.Data["User"] = models.User{Name: "Unknown"}
	c.Data["AppName"] = setting.AppName
	c.Data["AppVer"] = setting.AppVer
	c.Data["AppURL"] = setting.AppURL
	c.Data["Code"] = "2014031910370000009fff6782aadb2162b4a997acb69d4400888e0b9274657374"
	c.Data["ActiveCodeLives"] = setting.Service.ActiveCodeLives / 60
	c.Data["ResetPwdCodeLives"] = setting.Service.ResetPwdCodeLives / 60
	c.Data["CurDbValue"] = ""

	c.HTML(200, (c.Params("*")))
}
