// +build !cert

// Copyright 2009 The Go Authors. All rights reserved.
// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package cmd

import (
	"fmt"
	"os"

	"github.com/codegangsta/cli"
)

var CmdCert = cli.Command{
	Name:  "cert",
	Usage: "Generate self-signed certificate",
	Description: `Generate a self-signed X.509 certificate for a TLS server. 
Outputs to 'cert.pem' and 'key.pem' and will overwrite existing files.`,
	Action: runCert,
}

func runCert(ctx *cli.Context) {
	fmt.Println("Command cert not available, please use build tags 'cert' to rebuild.")
	os.Exit(1)
}
