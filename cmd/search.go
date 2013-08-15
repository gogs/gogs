// Copyright 2013 gopm authors.
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package cmd

import (
	"../doc"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

var CmdSearch = &Command{
	UsageLine: "search [keyword]",
	Short:     "search for package",
	Long: `
search packages

The search flags are:

	-s
		start a search service. This must be run before search a package

	-e
		search extactly, you should input an exactly package name as keyword
`,
}

func init() {
	CmdSearch.Run = runSearch
	CmdSearch.Flags = map[string]bool{
		"-s": false,
	}
}

func printSearchPrompt(flag string) {
	switch flag {
	case "-s":
		doc.ColorLog("[INFO] You enabled start a service.\n")
	case "-e":
		doc.ColorLog("[INFO] You enabled exactly search.\n")
	}
}

// search packages
func runSearch(cmd *Command, args []string) {
	// Check flags.
	num := checkFlags(cmd.Flags, args, printSearchPrompt)
	if num == -1 {
		return
	}
	args = args[num:]

	// Check length of arguments.
	if len(args) < 1 {
		doc.ColorLog("[ERROR] Please input package's keyword.\n")
		return
	}

	if cmd.Flags["-e"] {
		search(args[0], true)
	} else {
		search(args[0], false)
	}
}

/*
request local or remote search service to find packages according to keyword inputed
*/
func search(keyword string, isExactly bool) {
	url := "http://localhost:8991/search?"
	if isExactly {
		url = "http://localhost:8991/searche?"
	}
	resp, err := http.Get(url + keyword)
	if err != nil {
		doc.ColorLog(err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		contents, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			doc.ColorLog(err.Error())
			return
		}

		pkgs := make([]string, 0)
		err = json.Unmarshal(contents, &pkgs)
		if err != nil {
			doc.ColorLog(err.Error())
			return
		}
		for i, pkg := range pkgs {
			fmt.Println(i+1, pkg)
		}
	} else {
		doc.ColorLog(resp.Status)
	}
}
