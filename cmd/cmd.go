// Copyright 2013-2014 gopm authors.
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
	"os"
	"strings"

	"github.com/Unknwon/com"
	"github.com/codegangsta/cli"

	"github.com/gpmgo/gopm/doc"
	"github.com/gpmgo/gopm/log"
)

var (
	workDir string // The path of gopm was executed.
)

// setup initialize common environment for commands.
func setup(ctx *cli.Context) {
	var err error
	workDir, err = os.Getwd()
	if err != nil {
		log.Error("setup", "Fail to get work directory:")
		log.Fatal("", "\t"+err.Error())
	}

	log.PureMode = ctx.GlobalBool("noterm")
	log.Verbose = ctx.Bool("verbose")
}

// parseTarget returns "." when target is empty string.
func parseTarget(target string) string {
	if len(target) == 0 {
		target = "."
	}
	return target
}

// validPath checks if the information of the package is valid.
func validPath(info string) (string, string) {
	infos := strings.Split(info, ":")

	l := len(infos)
	switch {
	case l == 1:
		// For local imports.
		if com.IsFile(infos[0]) {
			return doc.LOCAL, infos[0]
		}
	case l == 2:
		switch infos[1] {
		case doc.TRUNK, doc.MASTER, doc.DEFAULT:
			infos[1] = ""
		}
		return infos[0], infos[1]
	}

	log.Error("", "Cannot parse dependency version:")
	log.Error("", "\t"+info)
	log.Help("Try 'gopm help get' to get more information")
	return "", ""
}

func versionSuffix(value string) string {
	if len(value) > 0 {
		return "." + value
	}
	return ""
}

func isSubpackage(rootPath, targetPath string) bool {
	return strings.HasSuffix(strings.Replace(workDir, "\\", "/", -1), rootPath) ||
		strings.HasPrefix(rootPath, targetPath)
}
