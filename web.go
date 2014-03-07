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
	"github.com/martini-contrib/sessions"

	"github.com/gogits/binding"

	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
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
	m.Use(base.InitContext())

	// TODO: should use other store because cookie store is not secure.
	store := sessions.NewCookieStore([]byte("secret123"))
	m.Use(sessions.Sessions("my_session", store))

	// Routers.
	m.Get("/", routers.Home)
	m.Any("/user/login", binding.BindIgnErr(auth.LogInForm{}), user.SignIn)
	m.Any("/user/logout", user.SignOut)
	m.Any("/user/sign_up", binding.BindIgnErr(auth.RegisterForm{}), user.SignUp)

	m.Get("/user/profile", user.Profile) // should be /username
	m.Any("/user/delete", user.Delete)
	m.Any("/user/publickey/add", user.AddPublicKey)
	m.Any("/user/publickey/list", user.ListPublicKey)
	m.Any("/repo/create", repo.Create)
	m.Any("/repo/delete", repo.Delete)
	m.Any("/repo/list", repo.List)

	listenAddr := fmt.Sprintf("%s:%s",
		utils.Cfg.MustValue("server", "HTTP_ADDR"),
		utils.Cfg.MustValue("server", "HTTP_PORT", "3000"))
	log.Info("Listen: %s", listenAddr)
	http.ListenAndServe(listenAddr, m)
}
