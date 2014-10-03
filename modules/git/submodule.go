// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package git

import (
	"strings"
)

type SubModule struct {
	Name string
	Url  string
}

// SubModuleFile represents a file with submodule type.
type SubModuleFile struct {
	*Commit

	refUrl string
	refId  string
}

func NewSubModuleFile(c *Commit, refUrl, refId string) *SubModuleFile {
	return &SubModuleFile{
		Commit: c,
		refUrl: refUrl,
		refId:  refId,
	}
}

// RefUrl guesses and returns reference URL.
func (sf *SubModuleFile) RefUrl() string {
	url := strings.TrimSuffix(sf.refUrl, ".git")

	// git://xxx/user/repo
	if strings.HasPrefix(url, "git://") {
		return "http://" + strings.TrimPrefix(url, "git://")
	}

	// http[s]://xxx/user/repo
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return url
	}

	// sysuser@xxx:user/repo
	i := strings.Index(url, "@")
	j := strings.LastIndex(url, ":")
	if i > -1 && j > -1 {
		return "http://" + url[i+1:j] + "/" + url[j+1:]
	}
	return url
}

// RefId returns reference ID.
func (sf *SubModuleFile) RefId() string {
	return sf.refId
}
