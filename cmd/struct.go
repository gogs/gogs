// Copyright (c) 2013 GPMGo Members. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/GPMGo/gopm/doc"
	"github.com/GPMGo/node"
)

var (
	Config  tomlConfig
	AppPath string // Application path.
)

var (
	LocalNodes   []*node.Node
	LocalBundles []*doc.Bundle
)

type tomlConfig struct {
	Title, Version string
	Lang           string `toml:"user_language"`
	AutoBackup     bool   `toml:"auto_backup"`
	Account        account
	AutoEnable     flagEnable `toml:"auto_enable"`
}

type flagEnable struct {
	Build, Install, Search, Check []string
}

type account struct {
	Username, Password  string
	Github_Access_Token string `toml:"github_access_token"`
}

// Use for i18n, key is prompt code, value is corresponding message.
var PromptMsg map[string]string

// A Command is an implementation of a go command
// like go build or go fix.
type Command struct {
	// Run runs the command.
	// The args are the arguments after the command name.
	Run func(cmd *Command, args []string)

	// UsageLine is the one-line usage message.
	// The first word in the line is taken to be the command name.
	UsageLine string

	// Short is the short description shown in the 'go help' output.
	Short string

	// Long is the long message shown in the 'go help <this-command>' output.
	Long string

	// Flag is a set of flags specific to this command.
	Flags map[string]bool
}

// Name returns the command's name: the first word in the usage line.
func (c *Command) Name() string {
	name := c.UsageLine
	i := strings.Index(name, " ")
	if i >= 0 {
		name = name[:i]
	}
	return name
}

func (c *Command) Usage() {
	fmt.Fprintf(os.Stderr, "usage: %s\n\n", c.UsageLine)
	fmt.Fprintf(os.Stderr, "%s\n", strings.TrimSpace(c.Long))
	os.Exit(2)
}

// Runnable reports whether the command can be run; otherwise
// it is a documentation pseudo-command such as importpath.
func (c *Command) Runnable() bool {
	return c.Run != nil
}

// executeCommand executes commands in command line.
func executeCommand(cmd string, args []string) {
	cmdExec := exec.Command(cmd, args...)
	stdout, err := cmdExec.StdoutPipe()
	if err != nil {
		fmt.Println(err)
	}
	stderr, err := cmdExec.StderrPipe()
	if err != nil {
		fmt.Println(err)
	}
	err = cmdExec.Start()
	if err != nil {
		fmt.Println(err)
	}
	go io.Copy(os.Stdout, stdout)
	go io.Copy(os.Stderr, stderr)
	cmdExec.Wait()
}
