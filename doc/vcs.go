// Copyright 2012 Gary Burd
//
// Copyright (c) 2013 GPMGo Members. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package doc

import (
	"bytes"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// TODO: specify with command line flag
const repoRoot = "/tmp/gddo"

var urlTemplates = []struct {
	re       *regexp.Regexp
	template string
	lineFmt  string
}{
	{
		regexp.MustCompile(`^git\.gitorious\.org/(?P<repo>[^/]+/[^/]+)$`),
		"https://gitorious.org/{repo}/blobs/{tag}/{dir}{0}",
		"#line%d",
	},
	{
		regexp.MustCompile(`^camlistore\.org/r/p/(?P<repo>[^/]+)$`),
		"http://camlistore.org/code/?p={repo}.git;hb={tag};f={dir}{0}",
		"#l%d",
	},
}

// lookupURLTemplate finds an expand() template, match map and line number
// format for well known repositories.
func lookupURLTemplate(repo, dir, tag string) (string, map[string]string, string) {
	if strings.HasPrefix(dir, "/") {
		dir = dir[1:] + "/"
	}
	for _, t := range urlTemplates {
		if m := t.re.FindStringSubmatch(repo); m != nil {
			match := map[string]string{
				"dir": dir,
				"tag": tag,
			}
			for i, name := range t.re.SubexpNames() {
				if name != "" {
					match[name] = m[i]
				}
			}
			return t.template, match, t.lineFmt
		}
	}
	return "", nil, ""
}

type vcsCmd struct {
	schemes  []string
	download func([]string, string, string) (string, string, error)
}

var vcsCmds = map[string]*vcsCmd{
	"git": &vcsCmd{
		schemes:  []string{"http", "https", "git"},
		download: downloadGit,
	},
}

var lsremoteRe = regexp.MustCompile(`(?m)^([0-9a-f]{40})\s+refs/(?:tags|heads)/(.+)$`)

func downloadGit(schemes []string, repo, savedEtag string) (string, string, error) {
	var p []byte
	var scheme string
	for i := range schemes {
		cmd := exec.Command("git", "ls-remote", "--heads", "--tags", schemes[i]+"://"+repo+".git")
		log.Println(strings.Join(cmd.Args, " "))
		var err error
		p, err = cmd.Output()
		if err == nil {
			scheme = schemes[i]
			break
		}
	}

	if scheme == "" {
		return "", "", NotFoundError{"VCS not found"}
	}

	tags := make(map[string]string)
	for _, m := range lsremoteRe.FindAllSubmatch(p, -1) {
		tags[string(m[2])] = string(m[1])
	}

	tag, commit, err := bestTag(tags, "master")
	if err != nil {
		return "", "", err
	}

	etag := scheme + "-" + commit

	if etag == savedEtag {
		return "", "", errNotModified
	}

	dir := path.Join(repoRoot, repo+".git")
	p, err = ioutil.ReadFile(path.Join(dir, ".git/HEAD"))
	switch {
	case err != nil:
		if err := os.MkdirAll(dir, 0777); err != nil {
			return "", "", err
		}
		cmd := exec.Command("git", "clone", scheme+"://"+repo, dir)
		log.Println(strings.Join(cmd.Args, " "))
		if err := cmd.Run(); err != nil {
			return "", "", err
		}
	case string(bytes.TrimRight(p, "\n")) == commit:
		return tag, etag, nil
	default:
		cmd := exec.Command("git", "fetch")
		log.Println(strings.Join(cmd.Args, " "))
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			return "", "", err
		}
	}

	cmd := exec.Command("git", "checkout", "--detach", "--force", commit)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		return "", "", err
	}

	return tag, etag, nil
}

var defaultTags = map[string]string{"git": "master", "hg": "default"}

func bestTag(tags map[string]string, defaultTag string) (string, string, error) {
	if commit, ok := tags["go1"]; ok {
		return "go1", commit, nil
	}
	if commit, ok := tags[defaultTag]; ok {
		return defaultTag, commit, nil
	}
	return "", "", NotFoundError{"Tag or branch not found."}
}

// expand replaces {k} in template with match[k] or subs[atoi(k)] if k is not in match.
func expand(template string, match map[string]string, subs ...string) string {
	var p []byte
	var i int
	for {
		i = strings.Index(template, "{")
		if i < 0 {
			break
		}
		p = append(p, template[:i]...)
		template = template[i+1:]
		i = strings.Index(template, "}")
		if s, ok := match[template[:i]]; ok {
			p = append(p, s...)
		} else {
			j, _ := strconv.Atoi(template[:i])
			p = append(p, subs[j]...)
		}
		template = template[i+1:]
	}
	p = append(p, template...)
	return string(p)
}

// checkImports checks package denpendencies.
func checkImports(absPath, importPath string) (importPkgs []string, err error) {
	dir, err := os.Open(absPath)
	if err != nil {
		return nil, err
	}
	defer dir.Close()

	// Get file info slice.
	fis, err := dir.Readdir(0)
	if err != nil {
		return nil, err
	}

	files := make([]*source, 0, 10)
	for _, fi := range fis {
		// Only handle files.
		if strings.HasSuffix(fi.Name(), ".go") {
			f, err := os.Open(absPath + fi.Name())
			if err != nil {
				return nil, err
			}

			fbytes := make([]byte, fi.Size())
			_, err = f.Read(fbytes)
			f.Close()
			//fmt.Println(d+fi.Name(), fi.Size(), n)
			if err != nil {
				return nil, err
			}

			files = append(files, &source{
				name: fi.Name(),
				data: fbytes,
			})
		}
	}

	// Check if has Go source files.
	if len(files) > 0 {
		w := &walker{ImportPath: importPath}
		importPkgs, err = w.build(files)
		if err != nil {
			return nil, err
		}
	}

	return importPkgs, err
}
