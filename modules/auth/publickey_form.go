// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"github.com/Unknwon/macaron"
	"github.com/macaron-contrib/i18n"

	"github.com/gogits/gogs/modules/middleware/binding"
)

type AddSSHKeyForm struct {
	SSHTitle string `form:"title" binding:"Required"`
	Content  string `form:"content" binding:"Required"`
}

func (f *AddSSHKeyForm) Validate(ctx *macaron.Context, errs *binding.Errors, l i18n.Locale) {
	validate(errs, ctx.Data, f, l)
}
