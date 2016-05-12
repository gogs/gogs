package cmd

import (
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"

	"github.com/Unknwon/cae/zip"
	"github.com/codegangsta/cli"
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/setting"
)

var CmdRestore = cli.Command{
	Name:  "restore",
	Usage: "Restore Gogs files and database",
	Description: `Restore compresses all related files and database into zip file.
It can be used for backup and capture Gogs server image to send to maintainer`,
	Action: runRestore,
	Flags: []cli.Flag{
		stringFlag("file, f", "", "Zipped dump to restore"),
		stringFlag("config, c", "", "Custom configuration file path"),
		boolFlag("verbose, v", "show process details"),
	},
}

func runRestore(ctx *cli.Context) {
	if !ctx.IsSet("file") {
		log.Fatalf("No dump-file set...")
	}
	if ctx.IsSet("config") {
		setting.CustomConf = ctx.String("config")
	}
	TmpWorkDir, err := ioutil.TempDir(os.TempDir(), "gogs-dump-")
	if err != nil {
		log.Fatalf("Fail to create tmp work directory: %v", err)
	}
	log.Printf("Creating tmp work dir: %s", TmpWorkDir)
	defer func() {
		log.Printf("Removing temp-dir: %s", TmpWorkDir)
		if err = os.RemoveAll(TmpWorkDir); err != nil {
			log.Fatal(err)
		}
	}()

	z, err := zip.Open(ctx.String("file"))
	if err != nil {
		log.Fatalf("Failed to open dump-file: %s\n", err)
	}
	defer z.Close()

	reposDump := path.Join(TmpWorkDir, "gogs-repo.zip")
	dbDump := path.Join(TmpWorkDir, "gogs-db.sql")

	log.Printf("Extracting repo-data to %s", TmpWorkDir)
	if err = z.ExtractTo(TmpWorkDir, "gogs-db.sql", "gogs-repo.zip"); err != nil {
		log.Fatalf("Failed to extract files to tmp-dir %s: %s", TmpWorkDir, err)
	}

	log.Printf("Extracting config to custom/conf/app.ini")
	if err = z.ExtractTo(".", "custom/conf/app.ini"); err != nil {
		log.Fatalf("Failer to extract config-file: %s", err)
	}

	log.Printf("Custom Conf at custom/conf/app.ini")
	setting.CustomConf = "custom/conf/app.ini"

	cwd, _ := os.Getwd()
	log.Printf("Restoring Logs %s", cwd)
	if err = z.ExtractTo(cwd, z.List("log")...); err != nil {
		log.Fatalf("Failed to extract log-files to %s", cwd)
	}

	log.Print("Loading config")
	setting.NewContext()
	log.Print("Setting up DB")
	models.LoadConfigs()
	if err = models.SetEngine(); err != nil {
		log.Fatalf("Error setting up engine: %s", err)
	}

	x := models.GetEngine()
	if x == nil {
		log.Fatalf("DB Engine is broken")
	}
	dbFile, err := ioutil.ReadFile(dbDump)
	if err != nil {
		log.Fatalf("Can't open dbDump %s: %s", dbDump, err)
	}
	log.Printf("Restoring DB %s", models.DbCfg.Path)
	if _, err = x.Exec(string(dbFile[:])); err != nil {
		log.Fatalf("Error restoring DB: %s", err)
	}

	repos, err := zip.Open(reposDump)
	if err != nil {
		log.Fatalf("Can't open repo-dump %s: %s", reposDump, err)
	}
	defer repos.Close()

	repoPath, repoDir := filepath.Split(setting.RepoRootPath)
	log.Printf("Extracting repo-dump to %s", TmpWorkDir)
	if err = repos.ExtractTo(repoPath, repos.List(repoDir)...); err != nil {
		log.Fatalf("Can't extract repo-dump %s to %s: %s", reposDump, repoPath, err)
	}
}
