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

package doc

import (
	"os"
	"path"
	"strings"

	"github.com/Unknwon/com"
	"github.com/Unknwon/goconfig"

	"github.com/gpmgo/gopm/log"
)

const (
	GopmFileName = ".gopmfile"
	RawHomeDir   = "~/.gopm"
)

var (
	HomeDir        = "~/.gopm"
	LocalNodesFile = "/data/localnodes.list"
	LocalNodes     *goconfig.ConfigFile
)

func init() {
	hd, err := com.HomeDir()
	if err != nil {
		log.Error("", "Fail to get current user")
		log.Fatal("", err.Error())
	}

	HomeDir = strings.Replace(RawHomeDir, "~", hd, -1)

	LoadLocalNodes()
}

func NewGopmfile(dirPath string) *goconfig.ConfigFile {
	gf, err := goconfig.LoadConfigFile(path.Join(dirPath, GopmFileName))
	if err != nil {
		log.Error("", "Fail to load gopmfile")
		log.Fatal("", err.Error())
	}
	return gf
}

var PackageNameList map[string]string

func LoadPkgNameList(filePath string) {
	PackageNameList = make(map[string]string)

	// If file does not exist, simply ignore.
	if !com.IsFile(filePath) {
		return
	}

	data, err := com.ReadFile(filePath)
	if err != nil {
		log.Error("Package name list", "Fail to read file")
		log.Fatal("", err.Error())
	}

	pkgs := strings.Split(string(data), "\n")
	for _, line := range pkgs {
		infos := strings.Split(line, "=")
		if len(infos) != 2 {
			log.Error("", "Fail to parse package name: "+line)
			log.Fatal("", "Invalid package name information")
		}
		PackageNameList[strings.TrimSpace(infos[0])] =
			strings.TrimSpace(infos[1])
	}
}

func LoadLocalNodes() {
	if !com.IsDir(HomeDir + "/data") {
		os.MkdirAll(HomeDir+"/data", os.ModePerm)
	}

	if !com.IsFile(HomeDir + LocalNodesFile) {
		os.Create(HomeDir + LocalNodesFile)
	}

	var err error
	LocalNodes, err = goconfig.LoadConfigFile(HomeDir + LocalNodesFile)
	if err != nil {
		log.Error("Load node", "Fail to load localnodes.list")
		log.Fatal("", err.Error())
	}
}
