// Copyright (c) 2013 GPMGo Members. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/GPMGo/gopm/doc"
	"github.com/GPMGo/gopm/utils"
	"github.com/GPMGo/node"
)

var (
	isHasGit, isHasHg bool
	downloadCache     map[string]bool // Saves packages that have been downloaded.
	installGOPATH     string          // The GOPATH that packages are downloaded to.
)

var CmdInstall = &Command{
	UsageLine: "install [install flags] <packages|bundles|snapshots>",
}

func init() {
	downloadCache = make(map[string]bool)
	CmdInstall.Run = runInstall
	CmdInstall.Flags = map[string]bool{
		"-d": false,
		"-u": false,
		"-e": false,
		"-b": false,
		"-s": false,
	}
}

// printInstallPrompt prints prompt information to users to
// let them know what's going on.
func printInstallPrompt(flag string) {
	switch flag {
	case "-d":
		fmt.Printf(fmt.Sprintf("%s\n", PromptMsg["DownloadOnly"]))
	case "-u":
		fmt.Printf(fmt.Sprintf("%s\n", PromptMsg["ForceUpdate"]))
	case "-e":
		fmt.Printf(fmt.Sprintf("%s\n", PromptMsg["DownloadExDeps"]))
	}
}

// checkFlags checks if the flag exists with correct format.
func checkFlags(flags map[string]bool, enable []string, args []string, print func(string)) int {
	// Check auto-enable.
	for _, v := range enable {
		flags["-"+v] = true
		print("-" + v)
	}

	num := 0 // Number of valid flags, use to cut out.
	for i, f := range args {
		// Check flag prefix '-'.
		if !strings.HasPrefix(f, "-") {
			// Not a flag, finish check process.
			break
		}

		// Check if it a valid flag.
		if v, ok := flags[f]; ok {
			flags[f] = !v
			if !v {
				print(f)
			} else {
				fmt.Println("DISABLE: " + f)
			}
		} else {
			fmt.Printf(fmt.Sprintf("%s\n", PromptMsg["UnknownFlag"]), f)
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
	num := checkFlags(cmd.Flags, Config.AutoEnable.Install, args, printInstallPrompt)
	if num == -1 {
		return
	}
	args = args[num:]

	// Check length of arguments.
	if len(args) < 1 {
		fmt.Printf(fmt.Sprintf("%s\n", PromptMsg["NoPackage"]))
		return
	}

	// Check version control tools.
	// checkVCSTool() // Since we don't user version control, we don't need to check this anymore.

	installGOPATH = utils.GetBestMatchGOPATH(AppPath)
	utils.ColorPrint(fmt.Sprintf(fmt.Sprintf("%s\n", PromptMsg["DownloadPath"]), installGOPATH))

	var nodes []*node.Node
	// Check if it is a bundle or snapshot.
	switch {
	case CmdInstall.Flags["-b"]:
		bundle := args[0]
		// Check local bundles.
		nodes = checkLocalBundles(bundle)
		if len(nodes) > 0 {
			// Check with users if continue.
			utils.ColorPrint(fmt.Sprintf(fmt.Sprintf("%s\n", PromptMsg["BundleInfo"]), bundle))
			for _, n := range nodes {
				fmt.Printf("[%s] -> %s: %s.\n", n.ImportPath, n.Type, n.Value)
			}
			fmt.Printf(fmt.Sprintf("%s\n", PromptMsg["ContinueDownload"]))
			var option string
			fmt.Fscan(os.Stdin, &option)
			if strings.ToLower(option) != "y" {
				os.Exit(0)
				return
			}
		} else {
			// Check from server.
			// TODO: api.GetBundleInfo()
			fmt.Println("Unable to find bundle, and we cannot check with server right now.")
		}
	case CmdInstall.Flags["-s"]:
		fmt.Println("gopm has not supported snapshot yet.")
		// TODO: api.GetSnapshotInfo()
	default:
		// Generate temporary nodes.
		nodes = make([]*node.Node, len(args))
		for i := range nodes {
			nodes[i] = new(node.Node)
			nodes[i].ImportPath = args[i]
		}
	}

	// Download packages.
	downloadPackages(nodes)

	// Check if need to install packages.
	if !CmdInstall.Flags["-d"] {
		// Remove old files.
		uninstallList := make([]string, 0, len(downloadCache))
		for k := range downloadCache {
			uninstallList = append(uninstallList, k)
		}
		removePackageFiles("", uninstallList)

		// Install packages all together.
		var cmdArgs []string
		cmdArgs = append(cmdArgs, "install")
		cmdArgs = append(cmdArgs, "<blank>")

		for k := range downloadCache {
			fmt.Printf(fmt.Sprintf("%s\n", PromptMsg["InstallStatus"]), k)
			cmdArgs[1] = k
			executeCommand("go", cmdArgs)
		}

		// Save local nodes to file.
		fw, err := os.Create(AppPath + "data/nodes.json")
		if err != nil {
			utils.ColorPrint(fmt.Sprintf(fmt.Sprintf("[ERROR] runInstall -> %s\n", PromptMsg["OpenFile"]), err))
			return
		}
		defer fw.Close()
		fbytes, err := json.MarshalIndent(&LocalNodes, "", "\t")
		if err != nil {
			utils.ColorPrint(fmt.Sprintf(fmt.Sprintf("[ERROR] runInstall -> %s\n", PromptMsg["ParseJSON"]), err))
			return
		}
		fw.Write(fbytes)
	}
}

// chekcDeps checks dependencies of nodes.
func chekcDeps(nodes []*node.Node) (depnodes []*node.Node) {
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
func checkLocalBundles(bundle string) (nodes []*node.Node) {
	for _, b := range LocalBundles {
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
func downloadPackages(nodes []*node.Node) {
	// Check all packages, they may be bundles, snapshots or raw packages path.
	for _, n := range nodes {
		// Check if it is a valid remote path.
		if utils.IsValidRemotePath(n.ImportPath) {
			if !downloadCache[n.ImportPath] {
				// Download package.
				nod, imports := downloadPackage(n)
				if len(imports) > 0 {
					// Need to download dependencies.
					// Generate temporary nodes.
					nodes := make([]*node.Node, len(imports))
					for i := range nodes {
						nodes[i] = new(node.Node)
						nodes[i].ImportPath = imports[i]
					}
					downloadPackages(nodes)
				}

				// Only save package information with specific commit.
				if nod != nil {
					// Save record in local nodes.
					saveNode(nod)
				}
			} else {
				fmt.Printf(fmt.Sprintf("%s\n", PromptMsg["SkipDownloaded"]), n.ImportPath)
			}
		} else {
			// Invalid import path.
			fmt.Printf(fmt.Sprintf("%s\n", PromptMsg["SkipInvalidPath"]), n.ImportPath)
		}
	}
}

// saveNode saves node into local nodes.
func saveNode(n *node.Node) {
	// Node dependencies list.
	n.Deps = nil

	// Check if this node exists.
	for i, v := range LocalNodes {
		if n.ImportPath == v.ImportPath {
			LocalNodes[i] = n
			return
		}
	}

	// Add new node.
	LocalNodes = append(LocalNodes, n)
}

// downloadPackage downloads package either use version control tools or not.
func downloadPackage(nod *node.Node) (*node.Node, []string) {
	fmt.Printf(fmt.Sprintf("%s\n", PromptMsg["DownloadStatus"]), nod.ImportPath)
	// Mark as donwloaded.
	downloadCache[nod.ImportPath] = true

	imports, err := pureDownload(nod)

	if err != nil {
		utils.ColorPrint(fmt.Sprintf(fmt.Sprintf("[ERROR] %s\n", PromptMsg["DownloadError"]), nod.ImportPath, err))
		return nil, nil
	}
	return nod, imports
}

// service represents a source code control service.
type service struct {
	pattern *regexp.Regexp
	prefix  string
	get     func(*http.Client, map[string]string, string, *node.Node, map[string]bool) ([]string, error)
}

// services is the list of source code control services handled by gopkgdoc.
var services = []*service{
	{doc.GithubPattern, "github.com/", doc.GetGithubDoc},
	{doc.GooglePattern, "code.google.com/", doc.GetGoogleDoc},
	{doc.BitbucketPattern, "bitbucket.org/", doc.GetBitbucketDoc},
	{doc.LaunchpadPattern, "launchpad.net/", doc.GetLaunchpadDoc},
}

// pureDownload downloads package without version control.
func pureDownload(nod *node.Node) ([]string, error) {
	for _, s := range services {
		if s.get == nil || !strings.HasPrefix(nod.ImportPath, s.prefix) {
			continue
		}
		m := s.pattern.FindStringSubmatch(nod.ImportPath)
		if m == nil {
			if s.prefix != "" {
				return nil,
					doc.NotFoundError{fmt.Sprintf("%s", PromptMsg["NotFoundError"])}
			}
			continue
		}
		match := map[string]string{"importPath": nod.ImportPath}
		for i, n := range s.pattern.SubexpNames() {
			if n != "" {
				match[n] = m[i]
			}
		}
		return s.get(doc.HttpClient, match, installGOPATH, nod, CmdInstall.Flags)
	}
	return nil, errors.New(fmt.Sprintf("%s", PromptMsg["NotFoundError"]))
}
