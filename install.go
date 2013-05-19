// Copyright (c) 2013 GPMGo Members. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"net/http"
	"os/exec"
	"regexp"
	"strings"

	"github.com/GPMGo/gpm/doc"
	"github.com/GPMGo/gpm/utils"
)

var (
	isHasGit, isHasHg bool
	downloadCache     map[string]bool // Saves packages that have downloaded.
)

var cmdInstall = &Command{
	UsageLine: "install [install flags] <packages|hash>",
}

func init() {
	downloadCache = make(map[string]bool)
	cmdInstall.Run = runInstall
	cmdInstall.Flags = map[string]bool{
		"-p": false,
		"-d": false,
		"-u": false,
	}
}

func runInstall(cmd *Command, args []string) {
	// Check if has flags.
	num := 0
	for i, f := range args {
		if strings.Index(f, "-") > -1 {
			// Deal with flags.
			if _, ok := cmdInstall.Flags[f]; ok {
				cmdInstall.Flags[f] = true
				printPrompt(f)
			} else {
				fmt.Printf("Unknown flag: %s.\n", f)
				return
			}
			num = i + 1
		}
	}
	// Cut out flag.
	args = args[num:]

	// Check length of arguments.
	if len(args) < 1 {
		fmt.Printf("Please list at least one package.\n")
		return
	}

	// Check version control tools.
	_, err := exec.LookPath("git")
	if err == nil {
		isHasGit = true
	}
	_, err = exec.LookPath("hg")
	if err == nil {
		isHasHg = true
	}

	// Install package(s).
	for _, p := range args {
		// Check if it is a hash string.
		// TODO

		// Check if it is vaild remote path.
		if !utils.IsValidRemotePath(p) {
			fmt.Printf("Invalid remote path: %s.\n", p)
		} else {
			downloadPackage(p, "")
		}
	}
}

func printPrompt(flag string) {
	switch flag {
	case "-p":
		fmt.Println("You enabled pure download.")
	case "-d":
		fmt.Println("You enabled download without installing.")
	}
}

// downloadPackage download package either use version control tools or not.
func downloadPackage(path, commit string) {
	// Check if use version control tools.
	switch {
	case !cmdInstall.Flags["-p"] &&
		((path[0] == 'g' && isHasGit) || (path[0] == 'c' && isHasHg)): // github.com, code.google.com
		args := checkGoGetFlags()
		args = append(args, path)
		fmt.Printf("Installing package: %s.\n", path)
		executeGoCommand(args)
	default: // Pure download.
		if !cmdInstall.Flags["-p"] {
			fmt.Printf("No version control tool available, pure download enabled!\n")
		}

		fmt.Printf("Downloading package: %s.\n", path)
		pkg, err := pureDownload(path, commit)
		if err != nil {
			fmt.Printf("Fail to download package(%s) with error: %s.\n", path, err)
		} else {
			fmt.Println(pkg)
			fmt.Printf("Checking imports(%s).\n", path)

			fmt.Printf("Installing package: %s.\n", path)
		}
	}
}

func checkGoGetFlags() (args []string) {
	args = append(args, "get")
	switch {
	case cmdInstall.Flags["-d"]:
		args = append(args, "-d")
		fallthrough
	case cmdInstall.Flags["-u"]:
		args = append(args, "-u")
	}

	return args
}

// service represents a source code control service.
type service struct {
	pattern *regexp.Regexp
	prefix  string
	get     func(*http.Client, map[string]string, string) (*doc.Package, error)
}

// services is the list of source code control services handled by gopkgdoc.
var services = []*service{
	{doc.GithubPattern, "github.com/", doc.GetGithubDoc},
	//{googlePattern, "code.google.com/", getGoogleDoc},
	//{bitbucketPattern, "bitbucket.org/", getBitbucketDoc},
	//{launchpadPattern, "launchpad.net/", getLaunchpadDoc},
}

// pureDownload downloads package without control control.
func pureDownload(path, commit string) (pinfo *doc.Package, err error) {
	for _, s := range services {
		if s.get == nil || !strings.HasPrefix(path, s.prefix) {
			continue
		}
		m := s.pattern.FindStringSubmatch(path)
		if m == nil {
			if s.prefix != "" {
				return nil, doc.NotFoundError{"Import path prefix matches known service, but regexp does not."}
			}
			continue
		}
		match := map[string]string{"importPath": path}
		for i, n := range s.pattern.SubexpNames() {
			if n != "" {
				match[n] = m[i]
			}
		}
		return s.get(doc.HttpClient, match, commit)
	}
	return nil, doc.ErrNoMatch
}
