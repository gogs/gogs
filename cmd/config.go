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
	"path"

	"github.com/Unknwon/goconfig"
	"github.com/codegangsta/cli"

	"github.com/gpmgo/gopm/doc"
	"github.com/gpmgo/gopm/log"
)

var CmdConfig = cli.Command{
	Name:  "config",
	Usage: "configurate gopm global settings",
	Description: `Command config configurates gopm global settings

gopm config github [client_id] [client_secret]
`,
	Action: runConfig,
	Flags: []cli.Flag{
		cli.BoolFlag{"verbose, v", "show process details"},
	},
}

func runConfig(ctx *cli.Context) {
	setup(ctx)

	if len(ctx.Args()) == 0 {
		log.Error("config", "Cannot start command:")
		log.Fatal("", "\tNo section specified")
	}

	switch ctx.Args()[0] {
	case "github":
		if len(ctx.Args()) < 3 {
			log.Error("config", "Cannot config section 'github'")
			log.Fatal("", "\tNot enough arguments for client_id and client_secret")
		}
		doc.Cfg.SetValue("github", "client_id", ctx.Args()[1])
		doc.Cfg.SetValue("github", "client_secret", ctx.Args()[2])
		goconfig.SaveConfigFile(doc.Cfg, path.Join(doc.HomeDir, doc.GOPM_CONFIG_FILE))
	}

	log.Success("SUCC", "config", "Command executed successfully!")
}
