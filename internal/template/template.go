package template

import (
	"encoding/json"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/transform"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/tool"
)

func ToUTF8WithErr(logger *logx.Logger, content []byte) (string, error) {
	charsetLabel, err := tool.DetectEncoding(logger, content)
	if err != nil {
		return "", err
	} else if charsetLabel == "UTF-8" {
		return string(content), nil
	}

	encoding, _ := charset.Lookup(charsetLabel)
	if encoding == nil {
		return string(content), errors.Newf("unknown encoding: %s", charsetLabel)
	}

	// If there is an error, we concatenate the nicely decoded part and the
	// original left over. This way we won't lose data.
	result, n, err := transform.String(encoding.NewDecoder(), string(content))
	if err != nil {
		result = result + string(content[n:])
	}

	return result, err
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
	} else if numLines == 1 || (numLines >= 2 && msgLines[1] == "") {
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

func ActionContent2Commits(logger *logx.Logger, act Actioner) *database.PushCommits {
	push := database.NewPushCommits()
	if err := json.Unmarshal([]byte(act.GetContent()), push); err != nil {
		logger.Error("Failed to unmarshal action content", "content", act.GetContent(), "error", err)
	}
	return push
}

// TODO(unknwon): Use url.Escape.
func EscapePound(str string) string {
	return strings.NewReplacer("%", "%25", "#", "%23", " ", "%20", "?", "%3F").Replace(str)
}
