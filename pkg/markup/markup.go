// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package markup

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/Unknwon/com"
	"golang.org/x/net/html"

	"github.com/gogits/gogs/pkg/setting"
	"github.com/gogits/gogs/pkg/tool"
)

// IsReadmeFile reports whether name looks like a README file based on its extension.
func IsReadmeFile(name string) bool {
	return strings.HasPrefix(strings.ToLower(name), "readme")
}

// IsIPythonNotebook reports whether name looks like a IPython notebook based on its extension.
func IsIPythonNotebook(name string) bool {
	return strings.HasSuffix(name, ".ipynb")
}

const (
	ISSUE_NAME_STYLE_NUMERIC      = "numeric"
	ISSUE_NAME_STYLE_ALPHANUMERIC = "alphanumeric"
)

var (
	// MentionPattern matches string that mentions someone, e.g. @Unknwon
	MentionPattern = regexp.MustCompile(`(\s|^|\W)@[0-9a-zA-Z-_\.]+`)

	// CommitPattern matches link to certain commit with or without trailing hash,
	// e.g. https://try.gogs.io/gogs/gogs/commit/d8a994ef243349f321568f9e36d5c3f444b99cae#diff-2
	CommitPattern = regexp.MustCompile(`(\s|^)https?.*commit/[0-9a-zA-Z]+(#+[0-9a-zA-Z-]*)?`)

	// IssueFullPattern matches link to an issue with or without trailing hash,
	// e.g. https://try.gogs.io/gogs/gogs/issues/4#issue-685
	IssueFullPattern = regexp.MustCompile(`(\s|^)https?.*issues/[0-9]+(#+[0-9a-zA-Z-]*)?`)
	// IssueNumericPattern matches string that references to a numeric issue, e.g. #1287
	IssueNumericPattern = regexp.MustCompile(`( |^|\()#[0-9]+\b`)
	// IssueAlphanumericPattern matches string that references to an alphanumeric issue, e.g. ABC-1234
	IssueAlphanumericPattern = regexp.MustCompile(`( |^|\()[A-Z]{1,10}-[1-9][0-9]*\b`)
	// CrossReferenceIssueNumericPattern matches string that references a numeric issue in a difference repository
	// e.g. gogits/gogs#12345
	CrossReferenceIssueNumericPattern = regexp.MustCompile(`( |^)[0-9a-zA-Z-_\.]+/[0-9a-zA-Z-_\.]+#[0-9]+\b`)

	// Sha1CurrentPattern matches string that represents a commit SHA, e.g. d8a994ef243349f321568f9e36d5c3f444b99cae
	// FIXME: this pattern matches pure numbers as well, right now we do a hack to check in RenderSha1CurrentPattern
	// by converting string to a number.
	Sha1CurrentPattern = regexp.MustCompile(`\b[0-9a-f]{40}\b`)
)

// FindAllMentions matches mention patterns in given content
// and returns a list of found user names without @ prefix.
func FindAllMentions(content string) []string {
	mentions := MentionPattern.FindAllString(content, -1)
	for i := range mentions {
		mentions[i] = mentions[i][strings.Index(mentions[i], "@")+1:] // Strip @ character
	}
	return mentions
}

// cutoutVerbosePrefix cutouts URL prefix including sub-path to
// return a clean unified string of request URL path.
func cutoutVerbosePrefix(prefix string) string {
	if len(prefix) == 0 || prefix[0] != '/' {
		return prefix
	}
	count := 0
	for i := 0; i < len(prefix); i++ {
		if prefix[i] == '/' {
			count++
		}
		if count >= 3+setting.AppSubURLDepth {
			return prefix[:i]
		}
	}
	return prefix
}

// RenderIssueIndexPattern renders issue indexes to corresponding links.
func RenderIssueIndexPattern(rawBytes []byte, urlPrefix string, metas map[string]string) []byte {
	urlPrefix = cutoutVerbosePrefix(urlPrefix)

	pattern := IssueNumericPattern
	if metas["style"] == ISSUE_NAME_STYLE_ALPHANUMERIC {
		pattern = IssueAlphanumericPattern
	}

	ms := pattern.FindAll(rawBytes, -1)
	for _, m := range ms {
		if m[0] == ' ' || m[0] == '(' {
			m = m[1:] // ignore leading space or opening parentheses
		}
		var link string
		if metas == nil {
			link = fmt.Sprintf(`<a href="%s/issues/%s">%s</a>`, urlPrefix, m[1:], m)
		} else {
			// Support for external issue tracker
			if metas["style"] == ISSUE_NAME_STYLE_ALPHANUMERIC {
				metas["index"] = string(m)
			} else {
				metas["index"] = string(m[1:])
			}
			link = fmt.Sprintf(`<a href="%s">%s</a>`, com.Expand(metas["format"], metas), m)
		}
		rawBytes = bytes.Replace(rawBytes, m, []byte(link), 1)
	}
	return rawBytes
}

// Note: this section is for purpose of increase performance and
// reduce memory allocation at runtime since they are constant literals.
var pound = []byte("#")

// RenderCrossReferenceIssueIndexPattern renders issue indexes from other repositories to corresponding links.
func RenderCrossReferenceIssueIndexPattern(rawBytes []byte, urlPrefix string, metas map[string]string) []byte {
	ms := CrossReferenceIssueNumericPattern.FindAll(rawBytes, -1)
	for _, m := range ms {
		if m[0] == ' ' || m[0] == '(' {
			m = m[1:] // ignore leading space or opening parentheses
		}

		delimIdx := bytes.Index(m, pound)
		repo := string(m[:delimIdx])
		index := string(m[delimIdx+1:])

		link := fmt.Sprintf(`<a href="%s%s/issues/%s">%s</a>`, setting.AppURL, repo, index, m)
		rawBytes = bytes.Replace(rawBytes, m, []byte(link), 1)
	}
	return rawBytes
}

// RenderSha1CurrentPattern renders SHA1 strings to corresponding links that assumes in the same repository.
func RenderSha1CurrentPattern(rawBytes []byte, urlPrefix string) []byte {
	return []byte(Sha1CurrentPattern.ReplaceAllStringFunc(string(rawBytes[:]), func(m string) string {
		if com.StrTo(m).MustInt() > 0 {
			return m
		}
		return fmt.Sprintf(`<a href="%s/commit/%s"><code>%s</code></a>`, urlPrefix, m, tool.ShortSHA1(string(m)))
	}))
}

// RenderSpecialLink renders mentions, indexes and SHA1 strings to corresponding links.
func RenderSpecialLink(rawBytes []byte, urlPrefix string, metas map[string]string) []byte {
	ms := MentionPattern.FindAll(rawBytes, -1)
	for _, m := range ms {
		m = m[bytes.Index(m, []byte("@")):]
		rawBytes = bytes.Replace(rawBytes, m,
			[]byte(fmt.Sprintf(`<a href="%s/%s">%s</a>`, setting.AppSubURL, m[1:], m)), -1)
	}

	rawBytes = RenderIssueIndexPattern(rawBytes, urlPrefix, metas)
	rawBytes = RenderCrossReferenceIssueIndexPattern(rawBytes, urlPrefix, metas)
	rawBytes = RenderSha1CurrentPattern(rawBytes, urlPrefix)
	return rawBytes
}

var (
	leftAngleBracket  = []byte("</")
	rightAngleBracket = []byte(">")
)

var noEndTags = []string{"input", "br", "hr", "img"}

// wrapImgWithLink warps link to standalone <img> tags.
func wrapImgWithLink(urlPrefix string, buf *bytes.Buffer, token html.Token) {
	// Extract "src" and "alt" attributes
	var src, alt string
	for i := range token.Attr {
		switch token.Attr[i].Key {
		case "src":
			src = token.Attr[i].Val
		case "alt":
			alt = token.Attr[i].Val
		}
	}

	// Skip in case the "src" is empty
	if len(src) == 0 {
		buf.WriteString(token.String())
		return
	}

	// Prepend repository base URL for internal links
	needPrepend := !isLink([]byte(src))
	if needPrepend {
		urlPrefix = strings.Replace(urlPrefix, "/src/", "/raw/", 1)
		if src[0] != '/' {
			urlPrefix += "/"
		}
	}

	buf.WriteString(`<a href="`)
	if needPrepend {
		buf.WriteString(urlPrefix)
		buf.WriteString(src)
	} else {
		buf.WriteString(src)
	}
	buf.WriteString(`">`)

	if needPrepend {
		src = strings.Replace(urlPrefix+string(src), " ", "%20", -1)
		buf.WriteString(`<img src="`)
		buf.WriteString(src)
		buf.WriteString(`"`)

		if len(alt) > 0 {
			buf.WriteString(` alt="`)
			buf.WriteString(alt)
			buf.WriteString(`"`)
		}

		buf.WriteString(`>`)

	} else {
		buf.WriteString(token.String())
	}

	buf.WriteString(`</a>`)
}

// postProcessHTML treats different types of HTML differently,
// and only renders special links for plain text blocks.
func postProcessHTML(rawHTML []byte, urlPrefix string, metas map[string]string) []byte {
	startTags := make([]string, 0, 5)
	buf := bytes.NewBuffer(nil)
	tokenizer := html.NewTokenizer(bytes.NewReader(rawHTML))

OUTER_LOOP:
	for html.ErrorToken != tokenizer.Next() {
		token := tokenizer.Token()
		switch token.Type {
		case html.TextToken:
			buf.Write(RenderSpecialLink([]byte(token.String()), urlPrefix, metas))

		case html.StartTagToken:
			tagName := token.Data

			if tagName == "img" {
				wrapImgWithLink(urlPrefix, buf, token)
				continue OUTER_LOOP
			}

			buf.WriteString(token.String())
			// If this is an excluded tag, we skip processing all output until a close tag is encountered.
			if strings.EqualFold("a", tagName) || strings.EqualFold("code", tagName) || strings.EqualFold("pre", tagName) {
				stackNum := 1
				for html.ErrorToken != tokenizer.Next() {
					token = tokenizer.Token()

					// Copy the token to the output verbatim
					buf.WriteString(token.String())

					// Stack number doesn't increate for tags without end tags.
					if token.Type == html.StartTagToken && !com.IsSliceContainsStr(noEndTags, token.Data) {
						stackNum++
					}

					// If this is the close tag to the outer-most, we are done
					if token.Type == html.EndTagToken {
						stackNum--
						if stackNum <= 0 && strings.EqualFold(tagName, token.Data) {
							break
						}
					}
				}
				continue OUTER_LOOP
			}

			if !com.IsSliceContainsStr(noEndTags, tagName) {
				startTags = append(startTags, tagName)
			}

		case html.EndTagToken:
			if len(startTags) == 0 {
				buf.WriteString(token.String())
				break
			}

			buf.Write(leftAngleBracket)
			buf.WriteString(startTags[len(startTags)-1])
			buf.Write(rightAngleBracket)
			startTags = startTags[:len(startTags)-1]
		default:
			buf.WriteString(token.String())
		}
	}

	if io.EOF == tokenizer.Err() {
		return buf.Bytes()
	}

	// If we are not at the end of the input, then some other parsing error has occurred,
	// so return the input verbatim.
	return rawHTML
}

type Type string

const (
	UNRECOGNIZED     Type = "unrecognized"
	MARKDOWN         Type = "markdown"
	ORG_MODE         Type = "orgmode"
	IPYTHON_NOTEBOOK Type = "ipynb"
)

// Detect returns best guess of a markup type based on file name.
func Detect(filename string) Type {
	switch {
	case IsMarkdownFile(filename):
		return MARKDOWN
	case IsOrgModeFile(filename):
		return ORG_MODE
	case IsIPythonNotebook(filename):
		return IPYTHON_NOTEBOOK
	default:
		return UNRECOGNIZED
	}
}

// Render takes a string or []byte and renders to HTML in given type of syntax with special links.
func Render(typ Type, input interface{}, urlPrefix string, metas map[string]string) []byte {
	var rawBytes []byte
	switch v := input.(type) {
	case []byte:
		rawBytes = v
	case string:
		rawBytes = []byte(v)
	default:
		panic(fmt.Sprintf("unrecognized input content type: %T", input))
	}

	urlPrefix = strings.TrimRight(strings.Replace(urlPrefix, " ", "%20", -1), "/")
	var rawHTML []byte
	switch typ {
	case MARKDOWN:
		rawHTML = RawMarkdown(rawBytes, urlPrefix)
	case ORG_MODE:
		rawHTML = RawOrgMode(rawBytes, urlPrefix)
	default:
		return rawBytes // Do nothing if syntax type is not recognized
	}

	rawHTML = postProcessHTML(rawHTML, urlPrefix, metas)
	return SanitizeBytes(rawHTML)
}
