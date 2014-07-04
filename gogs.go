// +build go1.2

// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Gogs(Go Git Service) is a Self Hosted Git Service in the Go Programming Language.
package main

import (
	"os"
	"runtime"

	"github.com/codegangsta/cli"

	"github.com/gogits/gogs/cmd"
	"github.com/gogits/gogs/modules/setting"
)

const APP_VER = "0.4.5.0704 Alpha"

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	setting.AppVer = APP_VER
}

func main() {
	app := cli.NewApp()
	app.Name = "Gogs"
	app.Usage = "Go Git Service"
	app.Version = APP_VER
	app.Commands = []cli.Command{
		cmd.CmdWeb,
		cmd.CmdServ,
		cmd.CmdUpdate,
		cmd.CmdFix,
		cmd.CmdDump,
	}
	app.Flags = append(app.Flags, []cli.Flag{}...)
	app.Run(os.Args)
}
