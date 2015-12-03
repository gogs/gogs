// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"log"
	"net/url"

	"github.com/codegangsta/cli"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/importer"
	"github.com/gogits/gogs/modules/setting"
)

var CmdImport = cli.Command{
	Name:  "import",
	Usage: "Import foreign project tracking service",
	Description: `Import connects to foreign project tracking service API and imports all common entities such as users, projects, issues into Gogs.
This can be used to migrate from other service to Gogs.`,
	Subcommands: []cli.Command{
		{
			Name:   "gitlab",
			Usage:  "import from GitLab",
			Action: importGitLab,
			Flags: []cli.Flag{
				stringFlag("url", "", "GitLab service base URL"),
				stringFlag("token", "", "private authentication token"),
			},
		},
		{
			Name:   "redmine",
			Usage:  "import from Redmine",
			Action: importGitLab,
			Flags: []cli.Flag{
				stringFlag("url", "", "Redmine service base URL"),
				stringFlag("token", "", "private authentication token"),
			},
		},
	},
	Flags: []cli.Flag{
		stringFlag("config, c", "custom/conf/app.ini", "Custom configuration file path"),
		boolFlag("verbose, v", "show process details"),
	},
}

func importGitLab(ctx *cli.Context) {
	baseUrl, err := url.Parse(ctx.String("url"))
	if err != nil {
		log.Fatal("Required --url parameter is missing or not valid URL")
	}
	if len(baseUrl.Path) > 0 {
		log.Fatal("Provided --url parameter must not contain path")
	}
	token := ctx.String("token")
	if len(token) == 0 {
		log.Fatal("Missing required --token parameter")
	}

	if ctx.IsSet("config") {
		setting.CustomConf = ctx.String("config")
	}
	setting.NewContext()
	models.LoadConfigs()
	models.SetEngine()

	if importer.ImportGitLab(baseUrl, token) == nil {
		log.Println("Finished importing!")
	} else {
		log.Println("Import failed!")
	}
}

func importRedmine(ctx *cli.Context) {
	baseUrl, err := url.Parse(ctx.String("url"))
	if err != nil {
		log.Fatal("Required --url parameter is missing or not valid URL")
	}
	if len(baseUrl.Path) > 0 {
		log.Fatal("Provided --url parameter must not contain path")
	}
	token := ctx.String("token")
	if len(token) == 0 {
		log.Fatal("Missing required --token parameter")
	}

	if ctx.IsSet("config") {
		setting.CustomConf = ctx.String("config")
	}
	setting.NewContext()
	models.LoadConfigs()
	models.SetEngine()

	if importer.ImportRedmine(baseUrl, token) == nil {
		log.Println("Finished importing!")
	} else {
		log.Println("Import failed!")
	}
}
