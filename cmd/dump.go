// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"log"
	"os"
	"path"

	"github.com/Unknwon/cae/zip"
	"github.com/codegangsta/cli"

	"github.com/gogits/gogs/modules/base"
)

var CmdDump = cli.Command{
	Name:  "dump",
	Usage: "Dump Gogs files except database",
	Description: `
Dump compresses all related files into zip file except database,
it can be used for backup and capture Gogs server image to send
to maintainer`,
	Action: runDump,
	Flags:  []cli.Flag{},
}

func runDump(*cli.Context) {
	base.NewConfigContext()

	log.Printf("Dumping local repositories...%s", base.RepoRootPath)
	zip.Verbose = false
	defer os.Remove("gogs-repo.zip")
	if err := zip.PackTo(base.RepoRootPath, "gogs-repo.zip", true); err != nil {
		log.Fatalf("Fail to dump local repositories: %v", err)
	}

	z, err := zip.Create("gogs-dump.zip")
	if err != nil {
		os.Remove("gogs-dump.zip")
		log.Fatalf("Fail to create gogs-dump.zip: %v", err)
	}

	execDir, _ := base.ExecDir()
	z.AddFile("gogs-repo.zip", path.Join(execDir, "gogs-repo.zip"))
	z.AddFile("custom/conf/app.ini", path.Join(execDir, "custom/conf/app.ini"))
	z.AddDir("log", path.Join(execDir, "log"))
	z.Close()

	log.Println("Finish dumping!")
}
