// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"net/http"

	"github.com/codegangsta/martini"
	"github.com/martini-contrib/render"

	"github.com/gogits/gogs/routers"
	"github.com/gogits/gogs/routers/user"
	"github.com/gogits/gogs/utils"
	"github.com/gogits/gogs/utils/log"
)

const APP_VER = "0.0.0.0218"

func init() {

}

func main() {
	log.Info("%s %s", utils.Cfg.MustValue("", "APP_NAME"), APP_VER)

	m := martini.Classic()

	// Middleware.
	m.Use(render.Renderer())

	// Routers.
	m.Get("/", routers.Dashboard)
	m.Get("/user/signin", user.SignIn)
	m.Any("/user/signup", user.SignUp)

	listenAddr := fmt.Sprintf("%s:%s",
		utils.Cfg.MustValue("server", "HTTP_ADDR"),
		utils.Cfg.MustValue("server", "HTTP_PORT", "3000"))
	log.Info("Listen: %s", listenAddr)
	http.ListenAndServe(listenAddr, m)
}
