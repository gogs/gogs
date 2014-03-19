// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package mailer

import (
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
)

func GetMailTmplData(user *models.User) map[interface{}]interface{} {
	data := make(map[interface{}]interface{}, 10)
	data["AppName"] = base.AppName
	data["AppVer"] = base.AppVer
	data["AppUrl"] = base.AppUrl
	data["AppLogo"] = base.AppLogo
	data["ActiveCodeLives"] = base.Service.ActiveCodeLives
	data["ResetPwdCodeLives"] = base.Service.ResetPwdCodeLives
	if user != nil {
		data["User"] = user
	}
	return data
}
