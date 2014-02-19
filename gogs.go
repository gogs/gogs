// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// gogs(Go Git Service) is a Go clone of Github.
package main

import (
	"os"
	"runtime"

	"github.com/codegangsta/cli"
)

// +build go1.1

// Test that go1.1 tag above is included in builds. main.go refers to this definition.
const go11tag = true

const APP_VER = "0.0.0.0219"

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}

func main() {
	app := cli.NewApp()
	app.Name = "Gogs"
	app.Usage = "Go Git Service"
	app.Version = APP_VER
	app.Commands = []cli.Command{
		CmdWeb,
		CmdServ,
	}
	app.Flags = append(app.Flags, []cli.Flag{
		cli.BoolFlag{"noterm", "disable color output"},
	}...)
	app.Run(os.Args)
	println("wo cao???")
}
