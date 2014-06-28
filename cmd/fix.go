// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/codegangsta/cli"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/setting"
)

var CmdFix = cli.Command{
	Name:        "fix",
	Usage:       "This command for upgrade from old version",
	Action:      runFix,
	Subcommands: fixCommands,
	Flags:       []cli.Flag{},
}

func runFix(ctx *cli.Context) {
}

var fixCommands = []cli.Command{
	{
		Name:  "location",
		Usage: "Change Gogs app location",
		Description: `Command location fixes location change of Gogs

gogs fix location <old Gogs path>
`,
		Action: runFixLocation,
	},
}

// rewriteAuthorizedKeys replaces old Gogs path to the new one.
func rewriteAuthorizedKeys(sshPath, oldPath, newPath string) error {
	fr, err := os.Open(sshPath)
	if err != nil {
		return err
	}
	defer fr.Close()

	tmpPath := sshPath + ".tmp"
	fw, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	defer fw.Close()

	oldPath = "command=\"" + oldPath + " serv"
	newPath = "command=\"" + newPath + " serv"
	buf := bufio.NewReader(fr)
	for {
		line, errRead := buf.ReadString('\n')
		line = strings.TrimSpace(line)

		if errRead != nil {
			if errRead != io.EOF {
				return errRead
			}

			// Reached end of file, if nothing to read then break,
			// otherwise handle the last line.
			if len(line) == 0 {
				break
			}
		}

		// Still finding the line, copy the line that currently read.
		if _, err = fw.WriteString(strings.Replace(line, oldPath, newPath, 1) + "\n"); err != nil {
			return err
		}

		if errRead == io.EOF {
			break
		}
	}

	if err = os.Remove(sshPath); err != nil {
		return err
	}
	return os.Rename(tmpPath, sshPath)
}

func rewriteUpdateHook(path, appPath string) error {
	rp := strings.NewReplacer("\\", "/", " ", "\\ ")
	if err := ioutil.WriteFile(path, []byte(fmt.Sprintf(models.TPL_UPDATE_HOOK,
		setting.ScriptType, rp.Replace(appPath))), os.ModePerm); err != nil {
		return err
	}
	return nil
}

func walkDir(rootPath, recPath, appPath string, depth int) error {
	depth++
	if depth > 3 {
		return nil
	} else if depth == 3 {
		if err := rewriteUpdateHook(path.Join(rootPath, "hooks/update"), appPath); err != nil {
			return err
		}
	}

	dir, err := os.Open(rootPath)
	if err != nil {
		return err
	}
	defer dir.Close()

	fis, err := dir.Readdir(0)
	if err != nil {
		return err
	}

	for _, fi := range fis {
		if strings.Contains(fi.Name(), ".DS_Store") {
			continue
		}

		relPath := path.Join(recPath, fi.Name())
		curPath := path.Join(rootPath, fi.Name())
		if fi.IsDir() {
			if err = walkDir(curPath, relPath, appPath, depth); err != nil {
				return err
			}
		}
	}
	return nil
}

func runFixLocation(ctx *cli.Context) {
	if len(ctx.Args()) != 1 {
		fmt.Println("Incorrect arguments number, expect 1")
		os.Exit(2)
	}

	execPath, _ := setting.ExecPath()

	oldPath := ctx.Args().First()
	fmt.Printf("Old location: %s\n", oldPath)
	fmt.Println("This command should be executed in the new Gogs path")
	fmt.Printf("Do you want to change Gogs app path from old location to:\n")
	fmt.Printf("-> %s?\n", execPath)
	fmt.Print("Press <enter> to continue, use <Ctrl+c> to exit.")
	fmt.Scanln()

	// Fix in authorized_keys file.
	sshPath := path.Join(models.SshPath, "authorized_keys")
	fmt.Printf("Fixing pathes in file: %s\n", sshPath)
	if err := rewriteAuthorizedKeys(sshPath, oldPath, execPath); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Fix position in gogs-repositories.
	setting.NewConfigContext()
	fmt.Printf("Fixing pathes in repositories: %s\n", setting.RepoRootPath)
	if err := walkDir(setting.RepoRootPath, "", execPath, 0); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println("Fix position finished!")
}
