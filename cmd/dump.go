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
	Flags:  []cli.Flag{},
}

func runDump(*cli.Context) {
	setting.NewConfigContext()
	models.LoadModelsConfig()
	models.SetEngine()

	log.Printf("Dumping local repositories...%s", setting.RepoRootPath)
	zip.Verbose = false
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
	z.AddFile("custom/conf/app.ini", path.Join(workDir, "custom/conf/app.ini"))
	z.AddDir("log", path.Join(workDir, "log"))
	if err = z.Close(); err != nil {
		os.Remove(fileName)
		log.Fatalf("Fail to save %s: %v", fileName, err)
	}

	log.Println("Finish dumping!")
}
