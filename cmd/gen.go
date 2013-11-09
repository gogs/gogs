// Copyright 2013 gopm authors.
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

package cmd

import (
	"os"

	"github.com/Unknwon/com"
	"github.com/Unknwon/goconfig"
	"github.com/codegangsta/cli"

	"github.com/gpmgo/gopm/doc"
	"github.com/gpmgo/gopm/log"
)

var CmdGen = cli.Command{
	Name:  "gen",
	Usage: "generate a gopmfile according current go project",
	Description: `Command gen gets dependencies and generates a gopmfile

gopm gen

Make sure you run this command in the root path of a go project.`,
	Action: runGen,
	Flags: []cli.Flag{
		cli.BoolFlag{"example", "download dependencies for example(s)"},
	},
}

// scan a directory and gen a gopm file
func runGen(ctx *cli.Context) {
	if !com.IsExist(".gopmfile") {
		os.Create(".gopmfile")
	}

	gf, err := goconfig.LoadConfigFile(".gopmfile")
	if err != nil {
		log.Error("Gen", "Fail to load gopmfile")
		log.Fatal("", err.Error())
	}

	curPath, err := os.Getwd()
	if err != nil {
		log.Error("Gen", "Fail to get work directory")
		log.Fatal("", err.Error())
	}

	// Get dependencies.
	imports := doc.GetAllImports([]string{curPath},
		gf.MustValue("target", "path"), ctx.Bool("example"))

	for _, p := range imports {
		if _, err := gf.GetValue("deps", doc.GetProjectPath(p)); err != nil {
			gf.SetValue("deps", doc.GetProjectPath(p), " ")
		}
	}

	err = goconfig.SaveConfigFile(gf, ".gopmfile")
	if err != nil {
		log.Error("Gen", "Fail to save gopmfile")
		log.Fatal("", err.Error())
	}

	log.Success("SUCC", "Gen", "Generate gopmfile successful!")
}
