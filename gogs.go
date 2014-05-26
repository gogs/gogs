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
	"github.com/gogits/gogs/modules/bin"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/setting"
)

const APP_VER = "0.3.6.0525 Alpha"

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	// go-bindata -ignore=\\.DS_Store -debug -o modules/bin/conf.go -pkg="bin" conf/...
	// Set and check if binary and static file version match.
	setting.AppVer = APP_VER
	data, err := bin.Asset("conf/VERSION")
	if err != nil {
		log.Fatal("Fail to read 'conf/VERSION': %v", err)
	}
	if string(data) != setting.AppVer {
		log.Fatal("Binary and static file version does not match, did you forget to recompile?")
	}
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
