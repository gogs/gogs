// Copyright 2013-2014 gogs authors.
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

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

const APP_VER = "0.0.0.0218"

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}

func main() {
	app := cli.NewApp()
	app.Name = "gogs"
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
}
