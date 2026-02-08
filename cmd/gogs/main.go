// Gogs is a painless self-hosted Git Service.
package main

import (
	"context"
	"os"

	"github.com/urfave/cli/v3"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
)

func init() {
	conf.App.Version = "0.15.0+dev"
}

func main() {
	cmd := &cli.Command{
		Name:    "Gogs",
		Usage:   "A painless self-hosted Git service",
		Version: conf.App.Version,
		Commands: []*cli.Command{
			&webCommand,
			&servCommand,
			&hookCommand,
			&adminCommand,
			&importCommand,
			&backupCommand,
			&restoreCommand,
		},
	}
	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal("Failed to start application: %v", err)
	}
}
