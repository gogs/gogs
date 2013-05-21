// Copyright 2011 Gary Burd
// Copyright 2013 Unknown
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
	"net/http"
	"path"
	"regexp"

	"github.com/Unknwon/gowalker/utils"
)

var (
	bitbucketPattern = regexp.MustCompile(`^bitbucket\.org/(?P<owner>[a-z0-9A-Z_.\-]+)/(?P<repo>[a-z0-9A-Z_.\-]+)(?P<dir>/[a-z0-9A-Z_.\-/]*)?$`)
	bitbucketEtagRe  = regexp.MustCompile(`^(hg|git)-`)
)

func getBitbucketDoc(client *http.Client, match map[string]string, savedEtag string) (*Package, error) {

	if m := bitbucketEtagRe.FindStringSubmatch(savedEtag); m != nil {
		match["vcs"] = m[1]
	} else {
		var repo struct {
			Scm string
		}
		if err := httpGetJSON(client, expand("https://api.bitbucket.org/1.0/repositories/{owner}/{repo}", match), &repo); err != nil {
			return nil, err
		}
		match["vcs"] = repo.Scm
	}

	tags := make(map[string]string)
	for _, nodeType := range []string{"branches", "tags"} {
		var nodes map[string]struct {
			Node string
		}
		if err := httpGetJSON(client, expand("https://api.bitbucket.org/1.0/repositories/{owner}/{repo}/{0}", match, nodeType), &nodes); err != nil {
			return nil, err
		}
		for t, n := range nodes {
			tags[t] = n.Node
		}
	}

	var err error
	match["tag"], match["commit"], err = bestTag(tags, defaultTags[match["vcs"]])
	if err != nil {
		return nil, err
	}

	// Check revision tag.
	etag := expand("{vcs}-{commit}", match)
	if etag == savedEtag {
		return nil, errNotModified
	}

	var node struct {
		Files []struct {
			Path string
		}
		Directories []string
	}

	if err := httpGetJSON(client, expand("https://api.bitbucket.org/1.0/repositories/{owner}/{repo}/src/{tag}{dir}/", match), &node); err != nil {
		return nil, err
	}

	// Get source file data.
	files := make([]*source, 0, 5)
	for _, f := range node.Files {
		_, name := path.Split(f.Path)
		if utils.IsDocFile(name) {
			files = append(files, &source{
				name:      name,
				browseURL: expand("https://bitbucket.org/{owner}/{repo}/src/{tag}/{0}", match, f.Path),
				rawURL:    expand("https://api.bitbucket.org/1.0/repositories/{owner}/{repo}/raw/{tag}/{0}", match, f.Path),
			})
		}
	}

	if len(files) == 0 && len(node.Directories) == 0 {
		return nil, NotFoundError{"Directory tree does not contain Go files and subdirs."}
	}

	// Fetch file from VCS.
	if err := fetchFiles(client, files, nil); err != nil {
		return nil, err
	}

	// Start generating data.
	w := &walker{
		lineFmt: "#cl-%d",
		pdoc: &Package{
			ImportPath:  match["importPath"],
			ProjectName: match["repo"],
			Etag:        etag,
			Dirs:        node.Directories,
		},
	}
	return w.build(files)
}
