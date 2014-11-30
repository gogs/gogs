// +build !cert

// Copyright 2009 The Go Authors. All rights reserved.
// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/codegangsta/cli"
)

var CmdCert = cli.Command{
	Name:  "cert",
	Usage: "Generate self-signed certificate",
	Description: `Generate a self-signed X.509 certificate for a TLS server. 
Outputs to 'cert.pem' and 'key.pem' and will overwrite existing files.`,
	Action: runCert,
	Flags: []cli.Flag{
		cli.StringFlag{"host", "", "Comma-separated hostnames and IPs to generate a certificate for", ""},
		cli.StringFlag{"ecdsa-curve", "", "ECDSA curve to use to generate a key. Valid values are P224, P256, P384, P521", ""},
		cli.IntFlag{"rsa-bits", 2048, "Size of RSA key to generate. Ignored if --ecdsa-curve is set", ""},
		cli.StringFlag{"start-date", "", "Creation date formatted as Jan 1 15:04:05 2011", ""},
		cli.DurationFlag{"duration", 365 * 24 * time.Hour, "Duration that certificate is valid for", ""},
		cli.BoolFlag{"ca", "whether this cert should be its own Certificate Authority", ""},
	},
}

func runCert(ctx *cli.Context) {
	fmt.Println("Command cert not available, please use build tags 'cert' to rebuild.")
	os.Exit(1)
}
