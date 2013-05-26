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
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"text/template"
	"unicode"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
	"github.com/GPMGo/gopm/cmd"
	"github.com/GPMGo/gopm/doc"
	"github.com/GPMGo/gopm/utils"
)

// Commands lists the available commands and help topics.
// The order here is the order in which they are printed by 'gpm help'.
var commands = []*cmd.Command{
	cmd.CmdBuild,
	cmd.CmdSearch,
	cmd.CmdInstall,
	cmd.CmdRemove,
	cmd.CmdCheck,
}

// getAppPath returns application execute path for current process.
func getAppPath() bool {
	// Look up executable in PATH variable.
	cmd.AppPath, _ = exec.LookPath(path.Base(os.Args[0]))
	// Check if run under $GOPATH/bin
	if !utils.IsExist(cmd.AppPath + "conf/") {
		paths := utils.GetGOPATH()
		for _, p := range paths {
			if utils.IsExist(p + "/src/github.com/GPMGo/gopm/") {
				cmd.AppPath = p + "/src/github.com/GPMGo/gopm/"
				break
			}
		}
	}

	if len(cmd.AppPath) == 0 {
		utils.ColorPrint("[ERROR] getAppPath ->[ Unable to indicate current execute path. ]\n")
		return false
	}

	cmd.AppPath = filepath.Dir(cmd.AppPath) + "/"
	if runtime.GOOS == "windows" {
		// Replace all '\' to '/'.
		cmd.AppPath = strings.Replace(cmd.AppPath, "\\", "/", -1)
	}

	doc.SetAppConfig(cmd.AppPath, cmd.Config.AutoBackup)
	return true
}

// loadPromptMsg loads prompt messages according to user language.
func loadPromptMsg(lang string) bool {
	cmd.PromptMsg = make(map[string]string)

	// Load prompt messages.
	f, err := os.Open(cmd.AppPath + "i18n/" + lang + "/prompt.txt")
	if err != nil {
		utils.ColorPrint(fmt.Sprintf("[ERROR] loadUsage -> Fail to load prompt messages[ %s ]\n", err))
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
			cmd.PromptMsg[p[:i]] = p[i+1:]
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
	f, err := os.Open(cmd.AppPath + "i18n/" + lang + "/usage.tpl")
	if err != nil {
		utils.ColorPrint(fmt.Sprintf(fmt.Sprintf("[ERROR] loadUsage -> %s\n", cmd.PromptMsg["LoadCommandUsage"]), "main", err))
		return false
	}
	defer f.Close()

	// Read main usages.
	fi, _ := f.Stat()
	usageBytes := make([]byte, fi.Size())
	f.Read(usageBytes)
	usageTemplate = string(usageBytes)

	// Load command usage.
	for _, command := range commands {
		f, err := os.Open(cmd.AppPath + "i18n/" + lang + "/usage_" + command.Name() + ".txt")
		if err != nil {
			utils.ColorPrint(fmt.Sprintf(fmt.Sprintf("[ERROR] loadUsage -> %s\n", cmd.PromptMsg["LoadCommandUsage"]), command.Name(), err))
			return false
		}
		defer f.Close()

		// Read usage.
		fi, _ := f.Stat()
		usageBytes := make([]byte, fi.Size())
		f.Read(usageBytes)
		usages := strings.Split(string(usageBytes), "|||")
		if len(usages) < 2 {
			utils.ColorPrint(fmt.Sprintf(
				fmt.Sprintf("[ERROR] loadUsage -> %s\n", cmd.PromptMsg["ReadCoammndUsage"]), command.Name()))
			return false
		}
		command.Short = usages[0]
		command.Long = usages[1]
	}

	return true
}

// loadLocalNodes loads nodes information from local file system.
func loadLocalNodes() bool {
	if !utils.IsExist(cmd.AppPath + "data/nodes.json") {
		os.MkdirAll(cmd.AppPath+"data/", os.ModePerm)
	} else {
		fr, err := os.Open(cmd.AppPath + "data/nodes.json")
		if err != nil {
			utils.ColorPrint(fmt.Sprintf(fmt.Sprintf("[ERROR] loadLocalNodes -> %s\n", cmd.PromptMsg["LoadLocalData"]), err))
			return false
		}
		defer fr.Close()

		err = json.NewDecoder(fr).Decode(&cmd.LocalNodes)
		if err != nil && err != io.EOF {
			utils.ColorPrint(fmt.Sprintf(fmt.Sprintf("[ERROR] loadLocalNodes -> %s\n", cmd.PromptMsg["ParseJSON"]), err))
			return false
		}
	}
	return true
}

// loadLocalBundles loads bundles from local file system.
func loadLocalBundles() bool {
	// Find all bundles.
	dir, err := os.Open(cmd.AppPath + "repo/bundles/")
	if err != nil {
		utils.ColorPrint(fmt.Sprintf(fmt.Sprintf("[ERROR] loadLocalBundles -> %s\n", cmd.PromptMsg["OpenFile"]), err))
		return false
	}
	defer dir.Close()

	fis, err := dir.Readdir(0)
	if err != nil {
		utils.ColorPrint(fmt.Sprintf(fmt.Sprintf("[ERROR] loadLocalBundles -> %s\n", cmd.PromptMsg["OpenFile"]), err))
		return false
	}

	for _, fi := range fis {
		// In case this folder contains unexpected directories.
		if !fi.IsDir() && strings.HasSuffix(fi.Name(), ".json") {
			fr, err := os.Open(cmd.AppPath + "repo/bundles/" + fi.Name())
			if err != nil {
				utils.ColorPrint(fmt.Sprintf(fmt.Sprintf("[ERROR] loadLocalBundles -> %s\n", cmd.PromptMsg["OpenFile"]), err))
				return false
			}

			bundle := new(doc.Bundle)
			err = json.NewDecoder(fr).Decode(bundle)
			fr.Close()
			if err != nil && err != io.EOF {
				utils.ColorPrint(fmt.Sprintf(fmt.Sprintf("[ERROR] loadLocalBundles -> %s\n", cmd.PromptMsg["ParseJSON"]), err))
				return false
			}

			// Make sure bundle name is not empty.
			if len(bundle.Name) == 0 {
				bundle.Name = fi.Name()[:strings.Index(fi.Name(), ".")]
			}

			cmd.LocalBundles = append(cmd.LocalBundles, bundle)
		}
	}
	return true
}

// We don't use init() to initialize
// bacause we need to get execute path in runtime.
func initialize() bool {
	// Try to have highest performance.
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Get application execute path.
	if !getAppPath() {
		return false
	}

	// Load configuration.
	if _, err := toml.DecodeFile(cmd.AppPath+"conf/gopm.toml", &cmd.Config); err != nil {
		fmt.Printf("initialize -> Fail to load configuration[ %s ]\n", err)
		return false
	}

	// Set github.com access token.
	doc.SetGithubCredentials(cmd.Config.Account.Github_Access_Token)

	// Load usages by language.
	if !loadUsage(cmd.Config.Lang) {
		return false
	}

	// Create bundle and snapshot directories.
	os.MkdirAll(cmd.AppPath+"repo/bundles/", os.ModePerm)
	os.MkdirAll(cmd.AppPath+"repo/snapshots/", os.ModePerm)
	// Create local tarball directories.
	os.MkdirAll(cmd.AppPath+"repo/tarballs/", os.ModePerm)

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
	fmt.Fprintf(os.Stderr, fmt.Sprintf("%s\n", cmd.PromptMsg["UnknownCommand"]), args[0])
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
var helpTemplate = `{{if .Runnable}}usage: gopm {{.UsageLine}}

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
		fmt.Fprintf(os.Stderr, "usage: gopm help command\n\nToo many arguments given.\n")
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

	fmt.Fprintf(os.Stderr, "Unknown help topic %#q.  Run 'gopm help'.\n", arg)
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
