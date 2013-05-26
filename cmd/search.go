// Copyright (c) 2013 GPMGo Members. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"fmt"
	"runtime"
	"strings"

	"github.com/GPMGo/gopm/doc"
	"github.com/GPMGo/gopm/utils"
)

var CmdSearch = &Command{
	UsageLine: "search [search flags] <keyword>",
}

func init() {
	CmdSearch.Run = runSearch
}

// printSearchPrompt prints prompt information to users to
// let them know what's going on.
func printSearchPrompt(flag string) {
	switch flag {

	}
}

func runSearch(cmd *Command, args []string) {
	// Check flags.
	num := checkFlags(cmd.Flags, Config.AutoEnable.Search, args, printSearchPrompt)
	if num == -1 {
		return
	}
	args = args[num:]

	// Check length of arguments.
	if len(args) < 1 {
		fmt.Printf(fmt.Sprintf("%s\n", PromptMsg["NoKeyword"]))
		return
	}

	// Search from server, and list results.
	results, err := doc.HttpGetBytes(doc.HttpClient, "http://gowalker.org/search?raw=true&q="+args[0], nil)
	if err != nil {
		utils.ColorPrint(fmt.Sprintf("[ERROR] runSearch -> [ %s ]\n", err))
		return
	}

	pkgsCache := make(map[string]string)
	paths := utils.GetGOPATH()
	pkgs := strings.Split(string(results), "|||")
	for _, p := range pkgs {
		i := strings.Index(p, "$")
		if i > -1 {
			// Do not display standard library.
			if !utils.IsGoRepoPath(p[:i]) {
				pkgsCache[utils.GetProjectPath(p[:i])] = p[i+1:]
			}
		}
	}

	if len(pkgsCache) == 0 {
		fmt.Printf("No result is available for keyword: %s.\n", args[0])
		return
	}

	isWindws := runtime.GOOS == "windows"
	var buf bytes.Buffer
	// Print split line for more clear look.
	splitLine := "<-----------------------------search results--------------------------->\n"
	if !isWindws {
		splitLine = strings.Replace(splitLine, "<", fmt.Sprintf(utils.PureStartColor, utils.Magenta)+"<", 1)
		splitLine = strings.Replace(splitLine, ">", ">"+utils.EndColor, 1)
	}
	buf.WriteString(splitLine)

	for k, v := range pkgsCache {
		// Package import path.
		buf.WriteString("-> " + k)
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
				buf.WriteString(installStr)
				break
			}
		}
		buf.WriteString("\n")

		if len(v) > 0 {
			buf.WriteString("        " + v + "\n") // Synopsis。
		}
	}

	resultStr := buf.String()

	if !isWindws {
		// Set color highlight.
		resultStr = strings.Replace(resultStr, args[0],
			fmt.Sprintf(utils.PureStartColor, utils.Yellow)+args[0]+utils.EndColor, -1)
	}

	fmt.Print(resultStr)
}
