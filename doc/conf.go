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
	"path/filepath"
	"strings"

	"github.com/Unknwon/com"
	"github.com/Unknwon/goconfig"

	"github.com/gpmgo/gopm/log"
)

const (
	GOPM_FILE_NAME = ".gopmfile"
	RawHomeDir     = "~/.gopm"
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
	LoadPkgNameList(HomeDir + "/data/pkgname.list")
}

// NewGopmfile loads gopmgile in given directory.
func NewGopmfile(dirPath string) *goconfig.ConfigFile {
	dirPath, _ = filepath.Abs(dirPath)
	gf, err := goconfig.LoadConfigFile(path.Join(dirPath, GOPM_FILE_NAME))
	if err != nil {
		log.Error("", "Fail to load gopmfile:")
		log.Fatal("", "\t"+err.Error())
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

func GetPkgFullPath(short string) string {
	name, ok := PackageNameList[short]
	if !ok {
		log.Error("", "Invalid package name")
		log.Error("", "It's not a invalid import path and no match in the package name list:")
		log.Fatal("", "\t"+short)
	}
	return name
}

func LoadLocalNodes() {
	if !com.IsDir(HomeDir + "/data") {
		os.MkdirAll(HomeDir+"/data", os.ModePerm)
	}

	if !com.IsFile(HomeDir + LocalNodesFile) {
		os.Create(HomeDir + LocalNodesFile)
	}

	var err error
	LocalNodes, err = goconfig.LoadConfigFile(path.Join(HomeDir + LocalNodesFile))
	if err != nil {
		log.Error("load node", "Fail to load localnodes.list")
		log.Fatal("", err.Error())
	}
}

func SaveLocalNodes() {
	if err := goconfig.SaveConfigFile(LocalNodes,
		path.Join(HomeDir+LocalNodesFile)); err != nil {
		log.Error("", "Fail to save localnodes.list:")
		log.Error("", "\t"+err.Error())
	}
}
