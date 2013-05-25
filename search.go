// Copyright (c) 2013 GPMGo Members. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/GPMGo/gopm/doc"
	"github.com/GPMGo/gopm/utils"
)

var cmdSearch = &Command{
	UsageLine: "search [search flags] <keyword>",
}

func init() {
	cmdSearch.Run = runSearch
}

// printSearchPrompt prints prompt information to users to
// let them know what's going on.
func printSearchPrompt(flag string) {
	switch flag {

	}
}

func runSearch(cmd *Command, args []string) {
	// Check flags.
	num := checkFlags(cmd.Flags, config.AutoEnable.Search, args, printSearchPrompt)
	if num == -1 {
		return
	}
	args = args[num:]

	// Check length of arguments.
	if len(args) < 1 {
		fmt.Printf(fmt.Sprintf("%s\n", promptMsg["NoKeyword"]))
		return
	}

	// Search from server, and list results.
	results, err := doc.HttpGetBytes(doc.HttpClient, "http://gowalker.org/search?raw=true&q="+args[0], nil)
	if err != nil {
		utils.ColorPrint(fmt.Sprintf("[ERROR] runSearch -> [ %s ]\n", err))
		return
	}

	resultStr := string(results)

	isWindws := runtime.GOOS == "windows"
	if !isWindws {
		// Set color highlight.
		resultStr = strings.Replace(resultStr, args[0],
			fmt.Sprintf(utils.PureStartColor, utils.Yellow)+args[0]+utils.EndColor, -1)
	}

	pkgsCache := make(map[string]string)
	paths := utils.GetGOPATH()
	pkgs := strings.Split(resultStr, "|||")
	for _, p := range pkgs {
		i := strings.Index(p, "$")
		if i > -1 {
			// Do not display standard library.
			if !utils.IsGoRepoPath(p[:i]) {
				pkgsCache[utils.GetProjectPath(p[:i])] = p[i+1:]
			}
		}
	}

	for k, v := range pkgsCache {
		fmt.Print("-> " + k) // Package import path.
		// Check if has been installed.
		for _, path := range paths {
			if checkIsExistWithVCS(path + "/src/" + k + "/") {
				installStr := " [Installed]"
				if !isWindws {
					installStr = strings.Replace(installStr, "[",
						fmt.Sprintf("[\033[%dm", utils.Green), 1)
					installStr = strings.Replace(installStr, "]",
						utils.EndColor+"]", 1)
				}
				fmt.Print(installStr)
				break
			}
		}
		fmt.Print("\n")

		if len(v) > 0 {
			fmt.Println("        " + v) // Synopsis。
		}
	}
}
