// Copyright (c) 2013 GPMGo Members. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/GPMGo/gpm/doc"
	"github.com/GPMGo/gpm/utils"
)

var (
	isHasGit, isHasHg bool
	downloadCache     map[string]bool // Saves packages that have downloaded.
	installGOPATH     string          // The GOPATH that packages are downloaded to.
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
		"-s": false,
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
	case "-s":
		fmt.Printf("You enabled download from sources.\n")
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

	installGOPATH = utils.GetBestMatchGOPATH(appPath)
	fmt.Printf("Packages will be downloaded to GOPATH(%s).\n", installGOPATH)

	// Generate temporary nodes.
	nodes := make([]*doc.Node, len(args))
	for i := range nodes {
		nodes[i] = new(doc.Node)
		nodes[i].ImportPath = args[i]
	}
	// Download packages.
	downloadPackages(nodes)

	if !cmdInstall.Flags["-d"] && cmdInstall.Flags["-p"] {
		// Install packages all together.
		var cmdArgs []string
		cmdArgs = append(cmdArgs, "install")
		cmdArgs = append(cmdArgs, "<blank>")

		for k := range downloadCache {
			fmt.Printf("Installing package: %s.\n", k)
			cmdArgs[1] = k
			executeGoCommand(cmdArgs)
		}

		// Save local nodes to file.
		fw, err := os.Create(appPath + "data/nodes.json")
		if err != nil {
			fmt.Println(err)
			return
		}
		defer fw.Close()
		fbytes, _ := json.MarshalIndent(&localNodes, "", "\t")
		fw.Write(fbytes)
	}

	fmt.Println("Well done.")
}

// chekcDeps checks dependencies of nodes.
func chekcDeps(nodes []*doc.Node) (depnodes []*doc.Node) {
	for _, n := range nodes {
		// Make sure it will not download all dependencies automatically.
		if len(n.Value) == 0 {
			n.Value = "B"
		}
		depnodes = append(depnodes, n)
		depnodes = append(depnodes, chekcDeps(n.Deps)...)
	}
	return depnodes
}

// checkLocalBundles checks if the bundle is in local file system.
func checkLocalBundles(bundle string) (nodes []*doc.Node) {
	for _, b := range localBundles {
		if bundle == b.Name {
			nodes = append(nodes, chekcDeps(b.Nodes)...)
			return nodes
		}
	}
	return nil
}

// downloadPackages downloads packages with certain commit,
// if the commit is empty string, then it downloads all dependencies,
// otherwise, it only downloada package with specific commit only.
func downloadPackages(nodes []*doc.Node) {
	// Check all packages, they may be bundles, snapshots or raw packages path.
	for _, n := range nodes {
		// Check if it is a bundle or snapshot.
		switch {
		case n.ImportPath[0] == 'B':
			// Check local bundles.
			bnodes := checkLocalBundles(n.ImportPath[1:])
			if len(nodes) > 0 {
				// Check with users if continue.
				fmt.Printf("Bundle(%s) contains following nodes:\n",
					n.ImportPath[1:])
				for _, bn := range bnodes {
					fmt.Printf("[%s] -> %s: %s.\n", bn.ImportPath, bn.Type, bn.Value)
				}
				fmt.Print("Continue to download?(Y/n).")
				var option string
				fmt.Fscan(os.Stdin, &option)
				if strings.ToLower(option) != "y" {
					os.Exit(0)
				}
				downloadPackages(bnodes)
			} else {
				// Check from server.
				// TODO: api.GetBundleInfo()
				fmt.Println("Unable to check with server right now.")
			}
		case n.ImportPath[0] == 'S':
			// TODO: api.GetSnapshotInfo()
		case utils.IsValidRemotePath(n.ImportPath):
			if !downloadCache[n.ImportPath] {
				// Download package.
				node, imports := downloadPackage(n)
				if len(imports) > 0 {
					// Need to download dependencies.
					// Generate temporary nodes.
					nodes := make([]*doc.Node, len(imports))
					for i := range nodes {
						nodes[i] = new(doc.Node)
						nodes[i].ImportPath = imports[i]
					}
					downloadPackages(nodes)
				}

				// Only save package information with specific commit.
				if node != nil {
					// Save record in local nodes.
					saveNode(node)
				}
			} else {
				fmt.Printf("Skipped downloaded package: %s.\n", n.ImportPath)
			}
		default:
			// Invalid import path.
			fmt.Printf("Skipped invalid import path: %s.\n", n.ImportPath)
		}
	}
}

// saveNode saves node into local nodes.
func saveNode(n *doc.Node) {
	// Check if this node exists.
	for i, v := range localNodes {
		if n.ImportPath == v.ImportPath {
			localNodes[i] = n
			return
		}
	}

	// Add new node.
	localNodes = append(localNodes, n)
}

// downloadPackage download package either use version control tools or not.
func downloadPackage(node *doc.Node) (*doc.Node, []string) {
	// Check if use version control tools.
	switch {
	case !cmdInstall.Flags["-p"] &&
		((node.ImportPath[0] == 'g' && isHasGit) || (node.ImportPath[0] == 'c' && isHasHg)): // github.com, code.google.com
		fmt.Printf("Installing package(%s) through 'go get'.\n", node.ImportPath)
		args := checkGoGetFlags()
		args = append(args, node.ImportPath)
		executeGoCommand(args)
		return nil, nil
	default: // Pure download.
		if !cmdInstall.Flags["-p"] {
			cmdInstall.Flags["-p"] = true
			fmt.Printf("No version control tool is available, pure download enabled!\n")
		}

		fmt.Printf("Downloading package: %s.\n", node.ImportPath)
		// Mark as donwloaded.
		downloadCache[node.ImportPath] = true

		imports, err := pureDownload(node)
		if err != nil {
			fmt.Printf("Fail to download package(%s) with error: %s.\n", node.ImportPath, err)
			return nil, nil
		}

		return node, imports
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
	get     func(*http.Client, map[string]string, string, *doc.Node, map[string]bool) ([]string, error)
}

// services is the list of source code control services handled by gopkgdoc.
var services = []*service{
	{doc.GithubPattern, "github.com/", doc.GetGithubDoc},
	{doc.GooglePattern, "code.google.com/", doc.GetGoogleDoc},
	{doc.BitbucketPattern, "bitbucket.org/", doc.GetBitbucketDoc},
	{doc.LaunchpadPattern, "launchpad.net/", doc.GetLaunchpadDoc},
}

// pureDownload downloads package without version control.
func pureDownload(node *doc.Node) ([]string, error) {
	for _, s := range services {
		if s.get == nil || !strings.HasPrefix(node.ImportPath, s.prefix) {
			continue
		}
		m := s.pattern.FindStringSubmatch(node.ImportPath)
		if m == nil {
			if s.prefix != "" {
				return nil,
					doc.NotFoundError{"Import path prefix matches known service, but regexp does not."}
			}
			continue
		}
		match := map[string]string{"importPath": node.ImportPath}
		for i, n := range s.pattern.SubexpNames() {
			if n != "" {
				match[n] = m[i]
			}
		}
		return s.get(doc.HttpClient, match, installGOPATH, node, cmdInstall.Flags)
	}
	return nil, doc.ErrNoMatch
}
