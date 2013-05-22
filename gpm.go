// Copyright (c) 2013 GPMGo Members. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// gpm(Go Package Manager) is a Go package manage tool for search, install, update and share packages in Go.

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"text/template"
	"unicode"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
	"github.com/GPMGo/gpm/doc"
	"github.com/GPMGo/gpm/utils"
)

var (
	config  tomlConfig
	appPath string // Application path.
)

var (
	localNodes   []*doc.Node
	localBundles []*doc.Bundle
)

// Use for i18n, key is prompt code, value is corresponding message.
var promptMsg map[string]string

type tomlConfig struct {
	Title, Version     string
	Username, Password string
	Lang               string `toml:"user_language"`
}

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

// Commands lists the available commands and help topics.
// The order here is the order in which they are printed by 'gpm help'.
var commands = []*Command{
	cmdBuild,
	cmdInstall,
}

// getAppPath returns application execute path for current process.
func getAppPath() bool {
	// Look up executable in PATH variable.
	appPath, _ = exec.LookPath("gpm")
	if len(appPath) == 0 {
		fmt.Printf("ERROR: getAppPath -> Unable to indicate current execute path.\n")
		return false
	}

	if runtime.GOOS == "windows" {
		// Replace all '\' to '/'.
		appPath = strings.Replace(filepath.Dir(appPath), "\\", "/", -1) + "/"
	}
	return true
}

// loadPromptMsg loads prompt messages according to user language.
func loadPromptMsg(lang string) bool {
	promptMsg = make(map[string]string)

	// Load prompt messages.
	f, err := os.Open(appPath + "i18n/" + lang + "/prompt.txt")
	if err != nil {
		fmt.Printf("ERROR: loadUsage -> Fail to load prompt messages[ %s ]\n", err)
		return false
	}
	defer f.Close()

	// Read prompt messages.
	fi, _ := f.Stat()
	promptBytes := make([]byte, fi.Size())
	f.Read(promptBytes)
	promptStrs := strings.Split(string(promptBytes), "\n")
	for _, p := range promptStrs {
		i := strings.Index(p, "=")
		if i > -1 {
			promptMsg[p[:i]] = p[i+1:]
		}
	}
	return true
}

// loadUsage loads usage according to user language.
func loadUsage(lang string) bool {
	if !loadPromptMsg(lang) {
		return false
	}

	// Load main usage.
	f, err := os.Open(appPath + "i18n/" + lang + "/usage.tpl")
	if err != nil {
		fmt.Printf(fmt.Sprintf("ERROR: loadUsage -> %s\n", promptMsg["LoadCommandUsage"]), "main", err)
		return false
	}
	defer f.Close()

	// Read main usages.
	fi, _ := f.Stat()
	usageBytes := make([]byte, fi.Size())
	f.Read(usageBytes)
	usageTemplate = string(usageBytes)

	// Load command usage.
	for _, cmd := range commands {
		f, err := os.Open(appPath + "i18n/" + lang + "/usage_" + cmd.Name() + ".txt")
		if err != nil {
			fmt.Printf(fmt.Sprintf("ERROR: loadUsage -> %s\n", promptMsg["LoadCommandUsage"]), cmd.Name(), err)
			return false
		}
		defer f.Close()

		// Read usage.
		fi, _ := f.Stat()
		usageBytes := make([]byte, fi.Size())
		f.Read(usageBytes)
		usages := strings.Split(string(usageBytes), "|||")
		if len(usages) < 2 {
			fmt.Printf(
				fmt.Sprintf("ERROR: loadUsage -> %s\n", promptMsg["ReadCoammndUsage"]), cmd.Name())
			return false
		}
		cmd.Short = usages[0]
		cmd.Long = usages[1]
	}

	return true
}

// loadLocalNodes loads nodes information from local file system.
func loadLocalNodes() bool {
	if !utils.IsExist(appPath + "data/nodes.json") {
		os.MkdirAll(appPath+"data/", os.ModePerm)
	} else {
		fr, err := os.Open(appPath + "data/nodes.json")
		if err != nil {
			fmt.Printf(fmt.Sprintf("ERROR: loadLocalNodes -> %s\n", promptMsg["LoadLocalData"]), err)
			return false
		}
		defer fr.Close()

		err = json.NewDecoder(fr).Decode(&localNodes)
		if err != nil && err != io.EOF {
			fmt.Printf(fmt.Sprintf("ERROR: loadLocalNodes -> %s\n", promptMsg["ParseJSON"]), err)
			return false
		}
	}
	return true
}

// loadLocalBundles loads bundles from local file system.
func loadLocalBundles() bool {
	// Find all bundles.
	dir, err := os.Open(appPath + "repo/bundles/")
	if err != nil {
		fmt.Printf(fmt.Sprintf("ERROR: loadLocalBundles -> %s\n", promptMsg["OpenFile"]), err)
		return false
	}
	defer dir.Close()

	fis, err := dir.Readdir(0)
	if err != nil {
		fmt.Printf(fmt.Sprintf("ERROR: loadLocalBundles -> %s\n", promptMsg["OpenFile"]), err)
		return false
	}

	for _, fi := range fis {
		// In case this folder contains unexpected directories.
		if !fi.IsDir() && strings.HasSuffix(fi.Name(), ".json") {
			fr, err := os.Open(appPath + "repo/bundles/" + fi.Name())
			if err != nil {
				fmt.Printf(fmt.Sprintf("ERROR: loadLocalBundles -> %s\n", promptMsg["OpenFile"]), err)
				return false
			}

			bundle := new(doc.Bundle)
			err = json.NewDecoder(fr).Decode(bundle)
			fr.Close()
			if err != nil && err != io.EOF {
				fmt.Printf(fmt.Sprintf("ERROR: loadLocalBundles -> %s\n", promptMsg["ParseJSON"]), err)
				return false
			}

			// Make sure bundle name is not empty.
			if len(bundle.Name) == 0 {
				bundle.Name = fi.Name()[:strings.Index(fi.Name(), ".")]
			}

			localBundles = append(localBundles, bundle)
		}
	}
	return true
}

// We don't use init() to initialize
// bacause we need to get execute path in runtime.
func initialize() bool {
	// Get application execute path.
	if !getAppPath() {
		return false
	}

	// Load configuration.
	if _, err := toml.DecodeFile(appPath+"conf/gpm.toml", &config); err != nil {
		fmt.Printf("initialize -> Fail to load configuration[ %s ]\n", err)
		return false
	}

	// Load usages by language.
	if !loadUsage(config.Lang) {
		return false
	}

	// Create bundle and snapshot directories.
	os.MkdirAll(appPath+"repo/bundles/", os.ModePerm)
	os.MkdirAll(appPath+"repo/snapshots/", os.ModePerm)

	// Initialize local data.
	if !loadLocalNodes() || !loadLocalBundles() {
		return false
	}

	return true
}

func main() {
	// Initialization.
	if !initialize() {
		return
	}

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
	for _, cmd := range commands {
		if cmd.Name() == args[0] && cmd.Run != nil {
			cmd.Run(cmd, args[1:])
			exit()
			return
		}
	}

	// Uknown commands.
	fmt.Fprintf(os.Stderr, fmt.Sprintf("%s\n", promptMsg["UnknownCommand"]), args[0])
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

var usageTemplate string
var helpTemplate = `{{if .Runnable}}usage: gpm {{.UsageLine}}

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
		// not exit 2: succeeded at 'gpm help'.
		return
	}
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "usage: gpm help command\n\nToo many arguments given.\n")
		os.Exit(2) // failed at 'gpm help'
	}

	arg := args[0]

	for _, cmd := range commands {
		if cmd.Name() == arg {
			tmpl(os.Stdout, helpTemplate, cmd)
			// not exit 2: succeeded at 'go help cmd'.
			return
		}
	}

	fmt.Fprintf(os.Stderr, "Unknown help topic %#q.  Run 'gpm help'.\n", arg)
	os.Exit(2) // failed at 'go help cmd'
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

// executeGoCommand executes go commands.
func executeGoCommand(args []string) {
	cmdExec := exec.Command("go", args...)
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
