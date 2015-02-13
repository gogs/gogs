// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"log"
	"os"
	"path"
	"time"

	"github.com/Unknwon/cae/zip"
	"github.com/codegangsta/cli"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/setting"
)

var CmdDump = cli.Command{
	Name:  "dump",
	Usage: "Dump Gogs files and database",
	Description: `Dump compresses all related files and database into zip file.
It can be used for backup and capture Gogs server image to send to maintainer`,
	Action: runDump,
	Flags: []cli.Flag{
		cli.StringFlag{"config, c", "custom/conf/app.ini", "Custom configuration file path", ""},
		cli.BoolFlag{"verbose, v", "show process details", ""},
	},
}

func runDump(ctx *cli.Context) {
	if ctx.IsSet("config") {
		setting.CustomConf = ctx.String("config")
	}
	setting.NewConfigContext()
	models.LoadModelsConfig()
	models.SetEngine()

	log.Printf("Dumping local repositories...%s", setting.RepoRootPath)
	zip.Verbose = ctx.Bool("verbose")
	defer os.Remove("gogs-repo.zip")
	if err := zip.PackTo(setting.RepoRootPath, "gogs-repo.zip", true); err != nil {
		log.Fatalf("Fail to dump local repositories: %v", err)
	}

	log.Printf("Dumping database...")
	defer os.Remove("gogs-db.sql")
	if err := models.DumpDatabase("gogs-db.sql"); err != nil {
		log.Fatalf("Fail to dump database: %v", err)
	}

	fileName := fmt.Sprintf("gogs-dump-%d.zip", time.Now().Unix())
	log.Printf("Packing dump files...")
	z, err := zip.Create(fileName)
	if err != nil {
		os.Remove(fileName)
		log.Fatalf("Fail to create %s: %v", fileName, err)
	}

	workDir, _ := setting.WorkDir()
	z.AddFile("gogs-repo.zip", path.Join(workDir, "gogs-repo.zip"))
	z.AddFile("gogs-db.sql", path.Join(workDir, "gogs-db.sql"))
	z.AddDir("custom", path.Join(workDir, "custom"))
	z.AddDir("log", path.Join(workDir, "log"))
	// FIXME: SSH key file.
	if err = z.Close(); err != nil {
		os.Remove(fileName)
		log.Fatalf("Fail to save %s: %v", fileName, err)
	}

	log.Println("Finish dumping!")
}
