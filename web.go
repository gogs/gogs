// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"html/template"
	"net/http"

	"github.com/codegangsta/cli"
	"github.com/codegangsta/martini"
	"github.com/martini-contrib/render"

	"github.com/gogits/gogs/routers"
	"github.com/gogits/gogs/routers/repo"
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

var AppHelpers template.FuncMap = map[string]interface{}{
	"AppName": func() string {
		return utils.Cfg.MustValue("", "APP_NAME")
	},
}

func runWeb(*cli.Context) {
	log.Info("%s %s", utils.Cfg.MustValue("", "APP_NAME"), APP_VER)

	m := martini.Classic()

	// Middleware.
	m.Use(render.Renderer(render.Options{Funcs: []template.FuncMap{AppHelpers}}))

	// Routers.
	m.Get("/", routers.Dashboard)
	m.Any("/login", user.SignIn)
	m.Any("/user/signin", user.SignIn)

	m.Any("/sign-up", user.SignUp)
	m.Any("/user/signup", user.SignUp)

	m.Get("/user/profile", user.Profile) // should be /username
	m.Any("/user/delete", user.Delete)
	m.Any("/user/publickey/add", user.AddPublicKey)
	m.Any("/repo/create", repo.Create)
	m.Any("/repo/delete", repo.Delete)

	listenAddr := fmt.Sprintf("%s:%s",
		utils.Cfg.MustValue("server", "HTTP_ADDR"),
		utils.Cfg.MustValue("server", "HTTP_PORT", "3000"))
	log.Info("Listen: %s", listenAddr)
	http.ListenAndServe(listenAddr, m)
}
