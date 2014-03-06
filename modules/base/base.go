// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package base

import (
	"github.com/codegangsta/martini"
)

type (
	// Type TmplData represents data in the templates.
	TmplData map[string]interface{}
)

func InitContext() martini.Handler {
	return func(context martini.Context) {
		context.Map(TmplData{})
	}
}
