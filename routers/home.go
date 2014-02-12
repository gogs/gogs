// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package routers

import (
	"github.com/martini-contrib/render"
)

func Home(r render.Render) {
	r.HTML(200, "home", map[string]interface{}{})
}
