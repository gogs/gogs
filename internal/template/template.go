package template

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/transform"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/encodingx"
)

// ToUTF8WithErr converts the content to UTF-8 from its detected encoding, see
// encodingx.Detect for ansiCharset.
func ToUTF8WithErr(logger *logx.Logger, content []byte, ansiCharset string) (string, error) {
	charsetLabel := encodingx.Detect(logger, content, ansiCharset)
	if charsetLabel == "UTF-8" {
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

type Actioner interface {
	GetOpType() int
	GetActUserName() string
	GetRepoUserName() string
	GetRepoName() string
	GetRepoPath() string
	GetRepoLink(subpath string) string
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
