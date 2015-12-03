// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"os"
	"bufio"
	"log"
	"strings"
	"text/template"
	"github.com/codegangsta/cli"

	"github.com/gogits/gogs/modules/setting"
)

var CmdGenerate = cli.Command{
	Name:  "generate",
	Usage: "Generate self-signed certificate, apache and nginx configuration files",
	Description: `Generate configuration files to use a web server as a proxy for Gogs.`,
	Flags: []cli.Flag{
		cli.StringFlag{"config, c", "custom/conf/app.ini", "Custom configuration file path", ""},
	},
	Subcommands: []cli.Command{
		CmdCert,
		{
			Name: "apache",
			Usage: "generate Apache configuration file",
			Action: runApache,
			Flags: []cli.Flag{
				cli.StringFlag{"subpath", "", "Use a sub-path", ""},
			},
		},
		{
			Name: "nginx",
			Usage: "generate nginx configuration file",
			Action: runNginx,
			Flags: []cli.Flag{
				cli.StringFlag{"subpath", "", "Use a sub-path", ""},
			},
		},
	},
}

type ServerType int

const (
	APACHE ServerType = iota
	NGINX
)

const ApacheConfTemplate =
`<VirtualHost *:80>
    ServerName {{.Domain}}

    <Proxy *>
        Order allow,deny
        Allow from all
    </Proxy>

    ProxyPreserveHost On
    ProxyRequests off
    <Location {{.Subpath}}>
        ProxyPass        http://{{.Addr}}:{{.Port}}/
        ProxyPassReverse http://{{.Addr}}:{{.Port}}/
    </Location>
</VirtualHost>`

const NginxConfTemplate =
`server {
    listen 80;
    server_name {{.Domain}};

    location {{.Subpath}} {
        proxy_pass http://{{.Addr}}:{{.Port}};
    }
}`

var ServerTemplates = map[ServerType]string{
	APACHE: ApacheConfTemplate,
	NGINX: NginxConfTemplate,
}

const outFile = "gogs.conf"

func runApache(ctx *cli.Context) {
	genConfigFile(ctx, APACHE)
}

func runNginx(ctx *cli.Context) {
	genConfigFile(ctx, NGINX)
}

func genConfigFile(ctx *cli.Context, st ServerType) {
	if ctx.IsSet("config") {
		setting.CustomConf = ctx.String("config")
	}
	setting.NewConfigContext()

	tmpl, err := template.New("conf").Parse(ServerTemplates[st])
	if err != nil {
		log.Fatalf("Failed to create configuration template: %s", err)
	}

	confOut, err := os.Create(outFile)
	if err != nil {
		log.Fatalf("Failed to open %s for writing: %s", outFile, err)
	}

	w := bufio.NewWriter(confOut)

	type Params struct {
		Domain string
		Addr string
		Port string
		Subpath string
	}

	params := Params{
		Domain: setting.Domain,
		Addr: "127.0.0.1",
		Port: setting.HttpPort,
		Subpath: "/",
	}

	if setting.HttpAddr != "0.0.0.0" {
		params.Addr = setting.HttpAddr
	}

	if ctx.IsSet("subpath") {
		params.Subpath = ctx.String("subpath")
		if (!strings.HasPrefix(params.Subpath, "/")) {
			params.Subpath = "/" + params.Subpath
		}
	}

	err = tmpl.Execute(w, params)
	if err != nil {
		log.Fatalf("Failed to write configuration to %s: %s", outFile, err)
	}

	w.Flush()
	confOut.Close()
	log.Printf("Written %s", outFile)
}
