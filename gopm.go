// Copyright 2013 gopm authors.
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

// gopm(Go Package Manager) is a Go package manage tool for search, install, update and share packages in Go.
package main

import (
	"fmt"
	"github.com/Unknwon/com"
	"github.com/gpmgo/gopm/cmd"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"text/template"
	"unicode"
	"unicode/utf8"
)

// +build go1.1

// Test that go1.1 tag above is included in builds. main.go refers to this definition.
const go11tag = true
const APP_VER = "0.2.5.0827"

var (
	config map[string]interface{}
)

// Commands lists the available commands and help topics.
// The order here is the order in which they are printed by 'gopm help'.
var commands = []*cmd.Command{
	cmd.CmdGet,
	cmd.CmdSearch,
	cmd.CmdServe,
	cmd.CmdGen,
	/*
		cmdBuild,
		cmdClean,
		cmdDoc,
		cmdEnv,
		cmdFix,
		cmdFmt,
		cmdInstall,
		cmdList,
		cmdRun,
		cmdTest,
		cmdTool,
		cmdVersion,
		cmdVet,

		helpGopath,
		helpPackages,
		helpRemote,
		helpTestflag,
		helpTestfunc,*/
}

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}

func main() {
	// Check length of arguments.
	args := os.Args[1:]
	if len(args) < 1 {
		usage()
		return
	}

	// Show help documentation.
	if args[0] == "help" {
		help(args[1:])
		return
	}

	// Check commands and run.
	for _, comm := range commands {
		if comm.Name() == args[0] && comm.Run != nil {
			if comm.Name() != "serve" {
				err := cmd.AutoRun()
				if err == nil {
					comm.Run(comm, args[1:])
				} else {
					com.ColorLog("[ERRO] %v\n", err)
				}
			} else {
				comm.Run(comm, args[1:])
			}
			exit()
			return
		}
	}

	fmt.Fprintf(os.Stderr, "gopm: unknown subcommand %q\nRun 'gopm help' for usage.\n", args[0])
	setExitStatus(2)
	exit()
}

var exitStatus = 0
var exitMu sync.Mutex

func setExitStatus(n int) {
	exitMu.Lock()
	if exitStatus < n {
		exitStatus = n
	}
	exitMu.Unlock()
}

var usageTemplate = `gopm is a package manage tool for Go programming language.

Usage:

	gopm command [arguments]

The commands are:
{{range .}}{{if .Runnable}}
    {{.Name | printf "%-11s"}} {{.Short}}{{end}}{{end}}

Use "gopm help [command]" for more information about a command.

Additional help topics:
{{range .}}{{if not .Runnable}}
    {{.Name | printf "%-11s"}} {{.Short}}{{end}}{{end}}

Use "gopm help [topic]" for more information about that topic.

`

var helpTemplate = `{{if .Runnable}}usage: go {{.UsageLine}}

{{end}}{{.Long | trim}}
`

// tmpl executes the given template text on data, writing the result to w.
func tmpl(w io.Writer, text string, data interface{}) {
	t := template.New("top")
	t.Funcs(template.FuncMap{"trim": strings.TrimSpace, "capitalize": capitalize})
	template.Must(t.Parse(text))
	if err := t.Execute(w, data); err != nil {
		panic(err)
	}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToTitle(r)) + s[n:]
}

func printUsage(w io.Writer) {
	tmpl(w, usageTemplate, commands)
}

func usage() {
	printUsage(os.Stderr)
	os.Exit(2)
}

// help implements the 'help' command.
func help(args []string) {
	if len(args) == 0 {
		printUsage(os.Stdout)
		// not exit 2: succeeded at 'gopm help'.
		return
	}
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "usage: gopm help command\n\nToo many arguments given.\n")
		os.Exit(2) // failed at 'gopm help'
	}

	arg := args[0]

	for _, cmd := range commands {
		if cmd.Name() == arg {
			tmpl(os.Stdout, helpTemplate, cmd)
			// not exit 2: succeeded at 'gopm help cmd'.
			return
		}
	}

	fmt.Fprintf(os.Stderr, "Unknown help topic %#q.  Run 'gopm help'.\n", arg)
	os.Exit(2) // failed at 'gopm help cmd'
}

var atexitFuncs []func()

func atexit(f func()) {
	atexitFuncs = append(atexitFuncs, f)
}

func exit() {
	for _, f := range atexitFuncs {
		f()
	}
	os.Exit(exitStatus)
}
