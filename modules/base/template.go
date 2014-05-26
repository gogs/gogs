// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package base

import (
	"bytes"
	"container/list"
	"encoding/json"
	"fmt"
	"html/template"
	"runtime"
	"strings"
	"time"

	"github.com/gogits/gogs/modules/setting"
)

func Str2html(raw string) template.HTML {
	return template.HTML(raw)
}

func Range(l int) []int {
	return make([]int, l)
}

func List(l *list.List) chan interface{} {
	e := l.Front()
	c := make(chan interface{})
	go func() {
		for e != nil {
			c <- e.Value
			e = e.Next()
		}
		close(c)
	}()
	return c
}

func ShortSha(sha1 string) string {
	if len(sha1) == 40 {
		return sha1[:10]
	}
	return sha1
}

var mailDomains = map[string]string{
	"gmail.com": "gmail.com",
}

var TemplateFuncs template.FuncMap = map[string]interface{}{
	"GoVer": func() string {
		return runtime.Version()
	},
	"AppName": func() string {
		return setting.AppName
	},
	"AppVer": func() string {
		return setting.AppVer
	},
	"AppDomain": func() string {
		return setting.Domain
	},
	"CdnMode": func() bool {
		return setting.ProdMode && !setting.OfflineMode
	},
	"LoadTimes": func(startTime time.Time) string {
		return fmt.Sprint(time.Since(startTime).Nanoseconds()/1e6) + "ms"
	},
	"AvatarLink": AvatarLink,
	"str2html":   Str2html,
	"TimeSince":  TimeSince,
	"FileSize":   FileSize,
	"Subtract":   Subtract,
	"Add": func(a, b int) int {
		return a + b
	},
	"ActionIcon": ActionIcon,
	"ActionDesc": ActionDesc,
	"DateFormat": DateFormat,
	"List":       List,
	"Mail2Domain": func(mail string) string {
		if !strings.Contains(mail, "@") {
			return "try.gogits.org"
		}

		suffix := strings.SplitN(mail, "@", 2)[1]
		domain, ok := mailDomains[suffix]
		if !ok {
			return "mail." + suffix
		}
		return domain
	},
	"SubStr": func(str string, start, length int) string {
		return str[start : start+length]
	},
	"DiffTypeToStr":     DiffTypeToStr,
	"DiffLineTypeToStr": DiffLineTypeToStr,
	"ShortSha":          ShortSha,
	"Oauth2Icon":        Oauth2Icon,
	"Oauth2Name":        Oauth2Name,
}

type Actioner interface {
	GetOpType() int
	GetActUserName() string
	GetActEmail() string
	GetRepoUserName() string
	GetRepoName() string
	GetBranch() string
	GetContent() string
}

// ActionIcon accepts a int that represents action operation type
// and returns a icon class name.
func ActionIcon(opType int) string {
	switch opType {
	case 1: // Create repository.
		return "plus-circle"
	case 5, 9: // Commit repository.
		return "arrow-circle-o-right"
	case 6: // Create issue.
		return "exclamation-circle"
	case 8: // Transfer repository.
		return "share"
	case 10: // Comment issue.
		return "comment"
	default:
		return "invalid type"
	}
}

const (
	TPL_CREATE_REPO    = `<a href="/user/%s">%s</a> created repository <a href="/%s">%s</a>`
	TPL_COMMIT_REPO    = `<a href="/user/%s">%s</a> pushed to <a href="/%s/src/%s">%s</a> at <a href="/%s">%s</a>%s`
	TPL_COMMIT_REPO_LI = `<div><img src="%s?s=16" alt="user-avatar"/> <a href="/%s/commit/%s" rel="nofollow">%s</a> %s</div>`
	TPL_CREATE_ISSUE   = `<a href="/user/%s">%s</a> opened issue <a href="/%s/issues/%s">%s#%s</a>
<div><img src="%s?s=16" alt="user-avatar"/> %s</div>`
	TPL_TRANSFER_REPO = `<a href="/user/%s">%s</a> transfered repository <code>%s</code> to <a href="/%s">%s</a>`
	TPL_PUSH_TAG      = `<a href="/user/%s">%s</a> pushed tag <a href="/%s/src/%s" rel="nofollow">%s</a> at <a href="/%s">%s</a>`
	TPL_COMMENT_ISSUE = `<a href="/user/%s">%s</a> commented on issue <a href="/%s/issues/%s">%s#%s</a>
<div><img src="%s?s=16" alt="user-avatar"/> %s</div>`
)

type PushCommit struct {
	Sha1        string
	Message     string
	AuthorEmail string
	AuthorName  string
}

type PushCommits struct {
	Len     int
	Commits []*PushCommit
}

// ActionDesc accepts int that represents action operation type
// and returns the description.
func ActionDesc(act Actioner) string {
	actUserName := act.GetActUserName()
	email := act.GetActEmail()
	repoUserName := act.GetRepoUserName()
	repoName := act.GetRepoName()
	repoLink := repoUserName + "/" + repoName
	branch := act.GetBranch()
	content := act.GetContent()
	switch act.GetOpType() {
	case 1: // Create repository.
		return fmt.Sprintf(TPL_CREATE_REPO, actUserName, actUserName, repoLink, repoName)
	case 5: // Commit repository.
		var push *PushCommits
		if err := json.Unmarshal([]byte(content), &push); err != nil {
			return err.Error()
		}
		buf := bytes.NewBuffer([]byte("\n"))
		for _, commit := range push.Commits {
			buf.WriteString(fmt.Sprintf(TPL_COMMIT_REPO_LI, AvatarLink(commit.AuthorEmail), repoLink, commit.Sha1, commit.Sha1[:7], commit.Message) + "\n")
		}
		if push.Len > 3 {
			buf.WriteString(fmt.Sprintf(`<div><a href="/%s/%s/commits/%s" rel="nofollow">%d other commits >></a></div>`, actUserName, repoName, branch, push.Len))
		}
		return fmt.Sprintf(TPL_COMMIT_REPO, actUserName, actUserName, repoLink, branch, branch, repoLink, repoLink,
			buf.String())
	case 6: // Create issue.
		infos := strings.SplitN(content, "|", 2)
		return fmt.Sprintf(TPL_CREATE_ISSUE, actUserName, actUserName, repoLink, infos[0], repoLink, infos[0],
			AvatarLink(email), infos[1])
	case 8: // Transfer repository.
		newRepoLink := content + "/" + repoName
		return fmt.Sprintf(TPL_TRANSFER_REPO, actUserName, actUserName, repoLink, newRepoLink, newRepoLink)
	case 9: // Push tag.
		return fmt.Sprintf(TPL_PUSH_TAG, actUserName, actUserName, repoLink, branch, branch, repoLink, repoLink)
	case 10: // Comment issue.
		infos := strings.SplitN(content, "|", 2)
		return fmt.Sprintf(TPL_COMMENT_ISSUE, actUserName, actUserName, repoLink, infos[0], repoLink, infos[0],
			AvatarLink(email), infos[1])
	default:
		return "invalid type"
	}
}

func DiffTypeToStr(diffType int) string {
	diffTypes := map[int]string{
		1: "add", 2: "modify", 3: "del",
	}
	return diffTypes[diffType]
}

func DiffLineTypeToStr(diffType int) string {
	switch diffType {
	case 2:
		return "add"
	case 3:
		return "del"
	case 4:
		return "tag"
	}
	return "same"
}

func Oauth2Icon(t int) string {
	switch t {
	case 1:
		return "fa-github-square"
	case 2:
		return "fa-google-plus-square"
	case 3:
		return "fa-twitter-square"
	case 4:
		return "fa-linux"
	case 5:
		return "fa-weibo"
	}
	return ""
}

func Oauth2Name(t int) string {
	switch t {
	case 1:
		return "GitHub"
	case 2:
		return "Google"
	case 3:
		return "Twitter"
	case 4:
		return "Tencent QQ"
	case 5:
		return "Weibo"
	}
	return ""
}
