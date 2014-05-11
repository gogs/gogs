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
	"github.com/gogits/gogs/modules/base"
)

const APP_VER = "0.3.3.0511 Alpha"

func init() {
	base.AppVer = APP_VER
	runtime.GOMAXPROCS(runtime.NumCPU())
}

func main() {
	app := cli.NewApp()
	app.Name = "Gogs"
	app.Usage = "Go Git Service"
	app.Version = APP_VER
	app.Commands = []cli.Command{
		cmd.CmdWeb,
		// cmd.CmdFix,
		cmd.CmdDump,
		cmd.CmdServ,
		cmd.CmdUpdate,
	}
	app.Flags = append(app.Flags, []cli.Flag{}...)
	app.Run(os.Args)
}
