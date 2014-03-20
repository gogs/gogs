// Copyright 2013-2014 gopm authors.
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
	"strings"

	"github.com/Unknwon/com"
	"github.com/Unknwon/goconfig"
	"github.com/codegangsta/cli"

	"github.com/gpmgo/gopm/doc"
	"github.com/gpmgo/gopm/log"
)

var CmdGen = cli.Command{
	Name:  "gen",
	Usage: "generate a gopmfile according current Go project",
	Description: `Command gen gets dependencies and generates a gopmfile

gopm gen

Make sure you run this command in the root path of a go project.`,
	Action: runGen,
	Flags: []cli.Flag{
		cli.BoolFlag{"example, e", "check dependencies for example(s)"},
		cli.BoolFlag{"verbose, v", "show process details"},
	},
}

var commonRes = []string{"views", "templates", "static", "public", "conf"}

func runGen(ctx *cli.Context) {
	setup(ctx)

	if !com.IsExist(".gopmfile") {
		os.Create(".gopmfile")
	}

	gf, err := goconfig.LoadConfigFile(".gopmfile")
	if err != nil {
		log.Error("gen", "Cannot load gopmfile:")
		log.Fatal("", "\t"+err.Error())
	}

	targetPath := parseTarget(gf.MustValue("target", "path"))
	// Get and set dependencies.
	imports := doc.GetAllImports([]string{workDir}, targetPath, ctx.Bool("example"), false)
	for _, p := range imports {
		p = doc.GetProjectPath(p)
		// Skip subpackage(s) of current project.
		if strings.HasSuffix(strings.Replace(workDir, "\\", "/", -1), p) ||
			strings.HasPrefix(p, targetPath) {
			continue
		}

		// Check if user specified the version.
		if value := gf.MustValue("deps", p); len(value) == 0 {
			gf.SetValue("deps", p, "")
		}
	}

	// Get and set resources.
	res := make([]string, 0, len(commonRes))
	for _, cr := range commonRes {
		if com.IsExist(cr) {
			res = append(res, cr)
		}
	}
	gf.SetValue("res", "include", strings.Join(res, "|"))

	err = goconfig.SaveConfigFile(gf, ".gopmfile")
	if err != nil {
		log.Error("gen", "Fail to save gopmfile:")
		log.Fatal("", "\t"+err.Error())
	}

	log.Success("SUCC", "gen", "Generate gopmfile successfully!")
}
