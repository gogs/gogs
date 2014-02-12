// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/codegangsta/martini"

	"github.com/gogits/gogs/routers"
)

const APP_VER = "0.0.0.0212"

func main() {
	m := martini.Classic()
	m.Get("/", routers.HomeGet)
	m.Run()
}
