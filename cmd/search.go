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

// import (
// 	"encoding/json"
// 	"fmt"
// 	"io/ioutil"
// 	"net/http"

// 	"github.com/Unknwon/com"
// )

// var CmdSearch = &Command{
// 	UsageLine: "search [keyword]",
// 	Short:     "search for package",
// 	Long: `
// search packages

// The search flags are:

// 	-e
// 		search extactly, you should input an exactly package name as keyword
// `,
// }

// func init() {
// 	CmdSearch.Run = runSearch
// 	CmdSearch.Flags = map[string]bool{
// 		"-e": false,
// 	}
// }

// func printSearchPrompt(flag string) {
// 	switch flag {
// 	case "-e":
// 		com.ColorLog("[INFO] You enabled exactly search.\n")
// 	}
// }

// // search packages
// func runSearch(cmd *Command, args []string) {

// 	// Check length of arguments.
// 	if len(args) < 1 {
// 		com.ColorLog("[ERROR] Please input package's keyword.\n")
// 		return
// 	}

// 	var host, port string
// 	host = "localhost"
// 	port = "8991"

// 	if cmd.Flags["-e"] {
// 		search(host, port, args[0], true)
// 	} else {
// 		search(host, port, args[0], false)
// 	}
// }

// type searchRes struct {
// 	Pkg  string
// 	Desc string
// }

// /*
// request local or remote search service to find packages according to keyword inputed
// */
// func search(host, port, keyword string, isExactly bool) {
// 	url := fmt.Sprintf("http://%v:%v/search?%v", host, port, keyword)
// 	if isExactly {
// 		url = fmt.Sprintf("http://%v:%v/searche?%v", host, port, keyword)
// 	}
// 	resp, err := http.Get(url)
// 	if err != nil {
// 		com.ColorLog(err.Error())
// 		return
// 	}
// 	defer resp.Body.Close()

// 	if resp.StatusCode == 200 {
// 		contents, err := ioutil.ReadAll(resp.Body)
// 		if err != nil {
// 			com.ColorLog(err.Error())
// 			return
// 		}

// 		pkgs := make([]searchRes, 0)
// 		err = json.Unmarshal(contents, &pkgs)
// 		if err != nil {
// 			com.ColorLog(err.Error())
// 			return
// 		}
// 		for i, pkg := range pkgs {
// 			fmt.Println(i+1, pkg.Pkg, "\t", pkg.Desc)
// 		}
// 	} else {
// 		com.ColorLog(resp.Status)
// 	}
// }
