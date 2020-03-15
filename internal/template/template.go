// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package template

import (
	"fmt"
	"html/template"
	"mime"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/editorconfig/editorconfig-core-go/v2"
	jsoniter "github.com/json-iterator/go"
	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/transform"
	log "unknwon.dev/clog/v2"

	"github.com/gogs/git-module"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/gitutil"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/tool"
)

var (
	funcMap     []template.FuncMap
	funcMapOnce sync.Once
)

// FuncMap returns a list of user-defined template functions.
func FuncMap() []template.FuncMap {
	funcMapOnce.Do(func() {
		funcMap = []template.FuncMap{map[string]interface{}{
			"BuildCommit": func() string {
				return conf.BuildCommit
			},
			"Year": func() int {
				return time.Now().Year()
			},
			"UseHTTPS": func() bool {
				return conf.Server.URL.Scheme == "https"
			},
			"AppName": func() string {
				return conf.App.BrandName
			},
			"AppSubURL": func() string {
				return conf.Server.Subpath
			},
			"AppURL": func() string {
				return conf.Server.ExternalURL
			},
			"AppVer": func() string {
				return conf.App.Version
			},
			"AppDomain": func() string {
				return conf.Server.Domain
			},
			"DisableGravatar": func() bool {
				return conf.Picture.DisableGravatar
			},
			"ShowFooterTemplateLoadTime": func() bool {
				return conf.Other.ShowFooterTemplateLoadTime
			},
			"LoadTimes": func(startTime time.Time) string {
				return fmt.Sprint(time.Since(startTime).Nanoseconds()/1e6) + "ms"
			},
			"AvatarLink":       tool.AvatarLink,
			"AppendAvatarSize": tool.AppendAvatarSize,
			"Safe":             Safe,
			"Sanitize":         bluemonday.UGCPolicy().Sanitize,
			"Str2HTML":         Str2HTML,
			"NewLine2br":       NewLine2br,
			"TimeSince":        tool.TimeSince,
			"RawTimeSince":     tool.RawTimeSince,
			"FileSize":         tool.FileSize,
			"Subtract":         tool.Subtract,
			"Add": func(a, b int) int {
				return a + b
			},
			"ActionIcon": ActionIcon,
			"DateFmtLong": func(t time.Time) string {
				return t.Format(time.RFC1123Z)
			},
			"DateFmtShort": func(t time.Time) string {
				return t.Format("Jan 02, 2006")
			},
			"SubStr": func(str string, start, length int) string {
				if len(str) == 0 {
					return ""
				}
				end := start + length
				if length == -1 {
					end = len(str)
				}
				if len(str) < end {
					return str
				}
				return str[start:end]
			},
			"Join":                  strings.Join,
			"EllipsisString":        tool.EllipsisString,
			"DiffFileTypeToStr":     DiffFileTypeToStr,
			"DiffLineTypeToStr":     DiffLineTypeToStr,
			"Sha1":                  Sha1,
			"ShortSHA1":             tool.ShortSHA1,
			"ActionContent2Commits": ActionContent2Commits,
			"EscapePound":           EscapePound,
			"RenderCommitMessage":   RenderCommitMessage,
			"ThemeColorMetaTag": func() string {
				return conf.UI.ThemeColorMetaTag
			},
			"FilenameIsImage": func(filename string) bool {
				mimeType := mime.TypeByExtension(filepath.Ext(filename))
				return strings.HasPrefix(mimeType, "image/")
			},
			"TabSizeClass": func(ec *editorconfig.Editorconfig, filename string) string {
				if ec != nil {
					def, err := ec.GetDefinitionForFilename(filename)
					if err == nil && def.TabWidth > 0 {
						return fmt.Sprintf("tab-size-%d", def.TabWidth)
					}
				}
				return "tab-size-8"
			},
			"InferSubmoduleURL": gitutil.InferSubmoduleURL,
		}}
	})
	return funcMap
}

func Safe(raw string) template.HTML {
	return template.HTML(raw)
}

func Str2HTML(raw string) template.HTML {
	return template.HTML(markup.Sanitize(raw))
}

// NewLine2br simply replaces "\n" to "<br>".
func NewLine2br(raw string) string {
	return strings.Replace(raw, "\n", "<br>", -1)
}

func Sha1(str string) string {
	return tool.SHA1(str)
}

func ToUTF8WithErr(content []byte) (error, string) {
	charsetLabel, err := tool.DetectEncoding(content)
	if err != nil {
		return err, ""
	} else if charsetLabel == "UTF-8" {
		return nil, string(content)
	}

	encoding, _ := charset.Lookup(charsetLabel)
	if encoding == nil {
		return fmt.Errorf("Unknown encoding: %s", charsetLabel), string(content)
	}

	// If there is an error, we concatenate the nicely decoded part and the
	// original left over. This way we won't loose data.
	result, n, err := transform.String(encoding.NewDecoder(), string(content))
	if err != nil {
		result = result + string(content[n:])
	}

	return err, result
}

// RenderCommitMessage renders commit message with special links.
func RenderCommitMessage(full bool, msg, urlPrefix string, metas map[string]string) string {
	cleanMsg := template.HTMLEscapeString(msg)
	fullMessage := string(markup.RenderIssueIndexPattern([]byte(cleanMsg), urlPrefix, metas))
	msgLines := strings.Split(strings.TrimSpace(fullMessage), "\n")
	numLines := len(msgLines)
	if numLines == 0 {
		return ""
	} else if !full {
		return msgLines[0]
	} else if numLines == 1 || (numLines >= 2 && len(msgLines[1]) == 0) {
		// First line is a header, standalone or followed by empty line
		header := fmt.Sprintf("<h3>%s</h3>", msgLines[0])
		if numLines >= 2 {
			fullMessage = header + fmt.Sprintf("\n<pre>%s</pre>", strings.Join(msgLines[2:], "\n"))
		} else {
			fullMessage = header
		}
	} else {
		// Non-standard git message, there is no header line
		fullMessage = fmt.Sprintf("<h4>%s</h4>", strings.Join(msgLines, "<br>"))
	}
	return fullMessage
}

type Actioner interface {
	GetOpType() int
	GetActUserName() string
	GetRepoUserName() string
	GetRepoName() string
	GetRepoPath() string
	GetRepoLink() string
	GetBranch() string
	GetContent() string
	GetCreate() time.Time
	GetIssueInfos() []string
}

// ActionIcon accepts a int that represents action operation type
// and returns a icon class name.
func ActionIcon(opType int) string {
	switch opType {
	case 1, 8: // Create and transfer repository
		return "repo"
	case 5: // Commit repository
		return "git-commit"
	case 6: // Create issue
		return "issue-opened"
	case 7: // New pull request
		return "git-pull-request"
	case 9: // Push tag
		return "tag"
	case 10: // Comment issue
		return "comment-discussion"
	case 11: // Merge pull request
		return "git-merge"
	case 12, 14: // Close issue or pull request
		return "issue-closed"
	case 13, 15: // Reopen issue or pull request
		return "issue-reopened"
	case 16: // Create branch
		return "git-branch"
	case 17, 18: // Delete branch or tag
		return "alert"
	case 19: // Fork a repository
		return "repo-forked"
	case 20, 21, 22: // Mirror sync
		return "repo-clone"
	default:
		return "invalid type"
	}
}

func ActionContent2Commits(act Actioner) *db.PushCommits {
	push := db.NewPushCommits()
	if err := jsoniter.Unmarshal([]byte(act.GetContent()), push); err != nil {
		log.Error("Unmarshal:\n%s\nERROR: %v", act.GetContent(), err)
	}
	return push
}

// TODO(unknwon): Use url.Escape.
func EscapePound(str string) string {
	return strings.NewReplacer("%", "%25", "#", "%23", " ", "%20", "?", "%3F").Replace(str)
}

func DiffFileTypeToStr(typ git.DiffFileType) string {
	return map[git.DiffFileType]string{
		git.DiffFileAdd:    "add",
		git.DiffFileChange: "modify",
		git.DiffFileDelete: "del",
		git.DiffFileRename: "rename",
	}[typ]
}

func DiffLineTypeToStr(typ git.DiffLineType) string {
	switch typ {
	case git.DiffLineAdd:
		return "add"
	case git.DiffLineDelete:
		return "del"
	case git.DiffLineSection:
		return "tag"
	}
	return "same"
}
