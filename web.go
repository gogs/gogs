// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"net/http"

	"github.com/codegangsta/cli"
	"github.com/codegangsta/martini"
	"github.com/martini-contrib/render"

	"github.com/gogits/gogs/routers"
	"github.com/gogits/gogs/routers/user"
	"github.com/gogits/gogs/utils"
	"github.com/gogits/gogs/utils/log"
)

var CmdWeb = cli.Command{
	Name:  "web",
	Usage: "just run",
	Description: `
gogs web`,
	Action: runWeb,
	Flags:  []cli.Flag{
	//cli.BoolFlag{"update, u", "update pakcage(s) and dependencies if any"},
	//cli.BoolFlag{"verbose, v", "show process details"},
	},
}

func runWeb(*cli.Context) {
	log.Info("%s %s", utils.Cfg.MustValue("", "APP_NAME"), APP_VER)

	m := martini.Classic()

	// Middleware.
	m.Use(render.Renderer())

	// Routers.
	m.Get("/", routers.Dashboard)
	m.Get("/user/signin", user.SignIn)
	m.Any("/user/signup", user.SignUp)
	m.Any("/user/delete", user.Delete)

	listenAddr := fmt.Sprintf("%s:%s",
		utils.Cfg.MustValue("server", "HTTP_ADDR"),
		utils.Cfg.MustValue("server", "HTTP_PORT", "3000"))
	log.Info("Listen: %s", listenAddr)
	http.ListenAndServe(listenAddr, m)
}
