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
	UsageLine: "install [install flags] <packages|bundles|snapshots>",
}

func init() {
	downloadCache = make(map[string]bool)
	cmdInstall.Run = runInstall
	cmdInstall.Flags = map[string]bool{
		"-p": false,
		"-d": false,
		"-u": false, // Flag for 'go get'.
		"-e": false,
	}
}

// printPrompt prints prompt information to users to
// let them know what's going on.
func printPrompt(flag string) {
	switch flag {
	case "-p":
		fmt.Printf("You enabled pure download.\n")
	case "-d":
		fmt.Printf("You enabled download without installing.\n")
	case "-e":
		fmt.Printf("You enabled download dependencies in example.\n")
	}
}

// checkFlags checks if the flag exists with correct format.
func checkFlags(args []string) int {
	num := 0 // Number of valid flags, use to cut out.
	for i, f := range args {
		// Check flag prefix '-'.
		if !strings.HasPrefix(f, "-") {
			// Not a flag, finish check process.
			break
		}

		// Check if it a valid flag.
		/* Here we use ok pattern to check it because
		this way can avoid same flag appears multiple times.*/
		if _, ok := cmdInstall.Flags[f]; ok {
			cmdInstall.Flags[f] = true
			printPrompt(f)
		} else {
			fmt.Printf("Unknown flag: %s.\n", f)
			return -1
		}
		num = i + 1
	}

	return num
}

// checkVCSTool checks if users have installed version control tools.
func checkVCSTool() {
	// git.
	if _, err := exec.LookPath("git"); err == nil {
		isHasGit = true
	}
	// hg.
	if _, err := exec.LookPath("hg"); err == nil {
		isHasHg = true
	}
	// svn.
}

func runInstall(cmd *Command, args []string) {
	// Check flags.
	num := checkFlags(args)
	if num == -1 {
		return
	}
	args = args[num:]

	// Check length of arguments.
	if len(args) < 1 {
		fmt.Printf("Please list at least one package/bundle/snapshot.\n")
		return
	}

	// Check version control tools.
	checkVCSTool()

	// Download packages.
	commits := make([]string, len(args))
	downloadPackages(args, commits)

	if !cmdInstall.Flags["d"] && cmdInstall.Flags["-p"] {
		// Install packages all together.
		fmt.Printf("Installing package: %s.\n")
	}

	fmt.Println("Well done.")
}

// downloadPackages downloads packages with certain commit,
// if the commit is empty string, then it downloads all dependencies,
// otherwise, it only downloada package with specific commit only.
func downloadPackages(pkgs, commits []string) {
	// Check all packages, they may be bundles, snapshots or raw packages path.
	for i, p := range pkgs {
		// Check if it is a bundle or snapshot.
		switch {
		case p[0] == 'B':
			// TODO: api.GetBundleInfo()
		case p[0] == 'S':
			// TODO: api.GetSnapshotInfo()
		case utils.IsValidRemotePath(p):
			if !downloadCache[p] {
				// Download package.
				pkg, imports := downloadPackage(p, commits[i])
				if len(imports) > 0 {
					// Need to download dependencies.
					tags := make([]string, len(imports))
					downloadPackages(imports, tags)
					continue
				}

				// Only save package information with specific commit.
				if pkg != nil {
					// Save record in local database.
					//fmt.Printf("Saved information: %s:%s.\n", pkg.ImportPath, pkg.Commit)
				}
			} else {
				fmt.Printf("Skipped downloaded package: %s.\n", p)
			}
		default:
			// Invalid import path.
			fmt.Printf("Skipped invalid import path: %s.\n", p)
		}
	}
}

// downloadPackage download package either use version control tools or not.
func downloadPackage(path, commit string) (pkg *doc.Package, imports []string) {
	// Check if use version control tools.
	switch {
	case !cmdInstall.Flags["-p"] &&
		((path[0] == 'g' && isHasGit) || (path[0] == 'c' && isHasHg)): // github.com, code.google.com
		fmt.Printf("Installing package(%s) through 'go get'.\n", path)
		args := checkGoGetFlags()
		args = append(args, path)
		executeGoCommand(args)
		return nil, nil
	default: // Pure download.
		if !cmdInstall.Flags["-p"] {
			cmdInstall.Flags["-p"] = true
			fmt.Printf("No version control tool is available, pure download enabled!\n")
		}

		fmt.Printf("Downloading package: %s.\n", path)
		// Mark as donwloaded.
		downloadCache[path] = true

		var err error
		pkg, imports, err = pureDownload(path, commit)
		if err != nil {
			fmt.Printf("Fail to download package(%s) with error: %s.\n", path, err)
			return nil, nil
		}

		//fmt.Println(pkg)
		return pkg, imports
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
	get     func(*http.Client, map[string]string, string, map[string]bool) (*doc.Package, []string, error)
}

// services is the list of source code control services handled by gopkgdoc.
var services = []*service{
	{doc.GithubPattern, "github.com/", doc.GetGithubDoc},
	{doc.GooglePattern, "code.google.com/", doc.GetGoogleDoc},
	//{bitbucketPattern, "bitbucket.org/", getBitbucketDoc},
	//{launchpadPattern, "launchpad.net/", getLaunchpadDoc},
}

// pureDownload downloads package without version control.
func pureDownload(path, commit string) (pinfo *doc.Package, imports []string, err error) {
	for _, s := range services {
		if s.get == nil || !strings.HasPrefix(path, s.prefix) {
			continue
		}
		m := s.pattern.FindStringSubmatch(path)
		if m == nil {
			if s.prefix != "" {
				return nil, nil,
					doc.NotFoundError{"Import path prefix matches known service, but regexp does not."}
			}
			continue
		}
		match := map[string]string{"importPath": path}
		for i, n := range s.pattern.SubexpNames() {
			if n != "" {
				match[n] = m[i]
			}
		}
		return s.get(doc.HttpClient, match, commit, cmdInstall.Flags)
	}
	return nil, nil, doc.ErrNoMatch
}
