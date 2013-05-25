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

	if runtime.GOOS != "windows" {
		// Set color highlight.
		resultStr = strings.Replace(resultStr, args[0], fmt.Sprintf(utils.PureStartColor, utils.Yellow)+args[0]+utils.EndColor, -1)
	}

	pkgs := strings.Split(resultStr, "|||")
	for _, p := range pkgs {
		i := strings.Index(p, "$")
		if i > -1 {
			fmt.Println("-> " + p[:i]) // Package import path.
			if len(p) > (i + 1) {
				fmt.Println("        " + p[i+1:]) // Synopsis。
			}
		}
	}
}
