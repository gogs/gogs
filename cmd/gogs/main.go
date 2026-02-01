// Gogs is a painless self-hosted Git Service.
package main

import (
	"os"

	"github.com/urfave/cli"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
)

func init() {
	conf.App.Version = "0.15.0+dev"
}

func main() {
	app := cli.NewApp()
	app.Name = "Gogs"
	app.Usage = "A painless self-hosted Git service"
	app.Version = conf.App.Version
	app.Commands = []cli.Command{
		webCommand,
		servCommand,
		hookCommand,
		certCommand,
		adminCommand,
		importCommand,
		backupCommand,
		restoreCommand,
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatal("Failed to start application: %v", err)
	}
}
