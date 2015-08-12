// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package base

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/russross/blackfriday"
	"golang.org/x/net/html"

	"github.com/gogits/gogs/modules/setting"
)

func isletter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isalnum(c byte) bool {
	return (c >= '0' && c <= '9') || isletter(c)
}

var validLinks = [][]byte{[]byte("http://"), []byte("https://"), []byte("ftp://"), []byte("mailto://")}

func isLink(link []byte) bool {
	for _, prefix := range validLinks {
		if len(link) > len(prefix) && bytes.Equal(bytes.ToLower(link[:len(prefix)]), prefix) && isalnum(link[len(prefix)]) {
			return true
		}
	}

	return false
}

func IsMarkdownFile(name string) bool {
	name = strings.ToLower(name)
	switch filepath.Ext(name) {
	case ".md", ".markdown", ".mdown", ".mkd":
		return true
	}
	return false
}

func IsTextFile(data []byte) (string, bool) {
	contentType := http.DetectContentType(data)
	if strings.Index(contentType, "text/") != -1 {
		return contentType, true
	}
	return contentType, false
}

func IsImageFile(data []byte) (string, bool) {
	contentType := http.DetectContentType(data)
	if strings.Index(contentType, "image/") != -1 {
		return contentType, true
	}
	return contentType, false
}

// IsReadmeFile returns true if given file name suppose to be a README file.
func IsReadmeFile(name string) bool {
	name = strings.ToLower(name)
	if len(name) < 6 {
		return false
	} else if len(name) == 6 {
		if name == "readme" {
			return true
		}
		return false
	}
	if name[:7] == "readme." {
		return true
	}
	return false
}

type CustomRender struct {
	blackfriday.Renderer
	urlPrefix string
}

func (options *CustomRender) Link(out *bytes.Buffer, link []byte, title []byte, content []byte) {
	if len(link) > 0 && !isLink(link) {
		if link[0] == '#' {
			// link = append([]byte(options.urlPrefix), link...)
		} else {
			link = []byte(path.Join(options.urlPrefix, string(link)))
		}
	}

	options.Renderer.Link(out, link, title, content)
}

func (options *CustomRender) Image(out *bytes.Buffer, link []byte, title []byte, alt []byte) {
	if len(link) > 0 && !isLink(link) {
		link = []byte(path.Join(strings.Replace(options.urlPrefix, "/src/", "/raw/", 1), string(link)))
	}

	options.Renderer.Image(out, link, title, alt)
}

var (
	MentionPattern     = regexp.MustCompile(`(\s|^)@[0-9a-zA-Z_\.]+`)
	commitPattern      = regexp.MustCompile(`(\s|^)https?.*commit/[0-9a-zA-Z]+(#+[0-9a-zA-Z-]*)?`)
	issueFullPattern   = regexp.MustCompile(`(\s|^)https?.*issues/[0-9]+(#+[0-9a-zA-Z-]*)?`)
	issueIndexPattern  = regexp.MustCompile(`( |^)#[0-9]+\b`)
	sha1CurrentPattern = regexp.MustCompile(`\b[0-9a-f]{40}\b`)
)

func RenderSpecialLink(rawBytes []byte, urlPrefix string) []byte {
	ms := MentionPattern.FindAll(rawBytes, -1)
	for _, m := range ms {
		m = bytes.TrimSpace(m)
		rawBytes = bytes.Replace(rawBytes, m,
			[]byte(fmt.Sprintf(`<a href="%s/%s">%s</a>`, setting.AppSubUrl, m[1:], m)), -1)
	}

	ms = commitPattern.FindAll(rawBytes, -1)
	for _, m := range ms {
		m = bytes.TrimSpace(m)
		i := strings.Index(string(m), "commit/")
		j := strings.Index(string(m), "#")
		if j == -1 {
			j = len(m)
		}
		rawBytes = bytes.Replace(rawBytes, m, []byte(fmt.Sprintf(
			` <code><a href="%s">%s</a></code>`, m, ShortSha(string(m[i+7:j])))), -1)
	}
	ms = issueFullPattern.FindAll(rawBytes, -1)
	for _, m := range ms {
		m = bytes.TrimSpace(m)
		i := strings.Index(string(m), "issues/")
		j := strings.Index(string(m), "#")
		if j == -1 {
			j = len(m)
		}
		rawBytes = bytes.Replace(rawBytes, m, []byte(fmt.Sprintf(
			` <a href="%s">#%s</a>`, m, ShortSha(string(m[i+7:j])))), -1)
	}
	rawBytes = RenderIssueIndexPattern(rawBytes, urlPrefix)
	rawBytes = RenderSha1CurrentPattern(rawBytes, urlPrefix)
	return rawBytes
}

func RenderSha1CurrentPattern(rawBytes []byte, urlPrefix string) []byte {
	ms := sha1CurrentPattern.FindAll(rawBytes, -1)
	for _, m := range ms {
		rawBytes = bytes.Replace(rawBytes, m, []byte(fmt.Sprintf(
			`<a href="%s/commit/%s"><code>%s</code></a>`, urlPrefix, m, ShortSha(string(m)))), -1)
	}
	return rawBytes
}

func RenderIssueIndexPattern(rawBytes []byte, urlPrefix string) []byte {
	ms := issueIndexPattern.FindAll(rawBytes, -1)
	for _, m := range ms {
		var space string
		m2 := m
		if m2[0] == ' ' {
			space = " "
			m2 = m2[1:]
		}
		rawBytes = bytes.Replace(rawBytes, m, []byte(fmt.Sprintf(`%s<a href="%s/issues/%s">%s</a>`,
			space, urlPrefix, m2[1:], m2)), 1)
	}
	return rawBytes
}

func RenderRawMarkdown(body []byte, urlPrefix string) []byte {
	htmlFlags := 0
	// htmlFlags |= blackfriday.HTML_USE_XHTML
	// htmlFlags |= blackfriday.HTML_USE_SMARTYPANTS
	// htmlFlags |= blackfriday.HTML_SMARTYPANTS_FRACTIONS
	// htmlFlags |= blackfriday.HTML_SMARTYPANTS_LATEX_DASHES
	// htmlFlags |= blackfriday.HTML_SKIP_HTML
	htmlFlags |= blackfriday.HTML_SKIP_STYLE
	// htmlFlags |= blackfriday.HTML_SKIP_SCRIPT
	// htmlFlags |= blackfriday.HTML_GITHUB_BLOCKCODE
	htmlFlags |= blackfriday.HTML_OMIT_CONTENTS
	// htmlFlags |= blackfriday.HTML_COMPLETE_PAGE
	renderer := &CustomRender{
		Renderer:  blackfriday.HtmlRenderer(htmlFlags, "", ""),
		urlPrefix: urlPrefix,
	}

	// set up the parser
	extensions := 0
	extensions |= blackfriday.EXTENSION_NO_INTRA_EMPHASIS
	extensions |= blackfriday.EXTENSION_TABLES
	extensions |= blackfriday.EXTENSION_FENCED_CODE
	extensions |= blackfriday.EXTENSION_AUTOLINK
	extensions |= blackfriday.EXTENSION_STRIKETHROUGH
	extensions |= blackfriday.EXTENSION_SPACE_HEADERS
	extensions |= blackfriday.EXTENSION_NO_EMPTY_LINE_BEFORE_BLOCK

	body = blackfriday.Markdown(body, renderer, extensions)
	return body
}

// PostProcessMarkdown treats different types of HTML differently,
// and only renders special links for plain text blocks.
func PostProcessMarkdown(rawHtml []byte, urlPrefix string) []byte {
	var buf bytes.Buffer
	tokenizer := html.NewTokenizer(bytes.NewReader(rawHtml))
	for html.ErrorToken != tokenizer.Next() {
		token := tokenizer.Token()
		switch token.Type {
		case html.TextToken:
			buf.Write(RenderSpecialLink([]byte(token.String()), urlPrefix))

		case html.StartTagToken:
			buf.WriteString(token.String())
			tagName := token.Data
			// If this is an excluded tag, we skip processing all output until a close tag is encountered.
			if strings.EqualFold("a", tagName) || strings.EqualFold("code", tagName) || strings.EqualFold("pre", tagName) {
				for html.ErrorToken != tokenizer.Next() {
					token = tokenizer.Token()
					// Copy the token to the output verbatim
					buf.WriteString(token.String())
					// If this is the close tag, we are done
					if html.EndTagToken == token.Type && strings.EqualFold(tagName, token.Data) {
						break
					}
				}
			}

		default:
			buf.WriteString(token.String())
		}
	}

	if io.EOF == tokenizer.Err() {
		return buf.Bytes()
	}

	// If we are not at the end of the input, then some other parsing error has occurred,
	// so return the input verbatim.
	return rawHtml
}

func RenderMarkdown(rawBytes []byte, urlPrefix string) []byte {
	result := RenderRawMarkdown(rawBytes, urlPrefix)
	result = PostProcessMarkdown(result, urlPrefix)
	result = Sanitizer.SanitizeBytes(result)
	return result
}

func RenderMarkdownString(raw, urlPrefix string) string {
	return string(RenderMarkdown([]byte(raw), urlPrefix))
}
