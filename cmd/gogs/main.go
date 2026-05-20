// Gogs is a painless self-hosted Git Service.
package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/cmd/gogs/internal/web"
	"gogs.io/gogs/internal/conf"
)

func init() {
	conf.App.Version = "0.15.0+dev"
}

var webCommand = cli.Command{
	Name:        "web",
	Usage:       "Start the web server",
	Description: "Serves the web interface, API, and HTTP Git endpoints.",
	Action: func(_ context.Context, cmd *cli.Command) error {
		var portOverride int
		if cmd.IsSet("port") {
			portOverride = cmd.Int("port")
		}
		return web.Run(configFromLineage(cmd), portOverride)
	},
	Flags: []cli.Flag{
		intFlag("port, p", 3000, "Alternative listening port to use"),
		stringFlag("config, c", filepath.Join(conf.CustomDir(), "conf", "app.ini"), "Custom configuration file path"),
	},
}

func main() {
	cmd := &cli.Command{
		Name:    "Gogs",
		Usage:   "The painless way to host your own Git service",
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
