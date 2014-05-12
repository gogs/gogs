// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package base

import (
	"bytes"
	"fmt"
	"net/http"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gogits/gfm"
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
	case ".md", ".markdown", ".mdown":
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

func IsReadmeFile(name string) bool {
	name = strings.ToLower(name)
	if len(name) < 6 {
		return false
	}
	if name[:6] == "readme" {
		return true
	}
	return false
}

type CustomRender struct {
	gfm.Renderer
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

var (
	MentionPattern    = regexp.MustCompile(`@[0-9a-zA-Z_]{1,}`)
	commitPattern     = regexp.MustCompile(`(\s|^)https?.*commit/[0-9a-zA-Z]+(#+[0-9a-zA-Z-]*)?`)
	issueFullPattern  = regexp.MustCompile(`(\s|^)https?.*issues/[0-9]+(#+[0-9a-zA-Z-]*)?`)
	issueIndexPattern = regexp.MustCompile(`#[0-9]+`)
)

func RenderSpecialLink(rawBytes []byte, urlPrefix string) []byte {
	buf := bytes.NewBufferString("")
	inCodeBlock := false
	codeBlockPrefix := []byte("```")
	lineBreak := []byte("\n")
	tab := []byte("\t")
	lines := bytes.Split(rawBytes, lineBreak)
	for _, line := range lines {
		if bytes.HasPrefix(line, codeBlockPrefix) {
			inCodeBlock = !inCodeBlock
		}

		if !inCodeBlock && !bytes.HasPrefix(line, tab) {
			ms := MentionPattern.FindAll(line, -1)
			for _, m := range ms {
				line = bytes.Replace(line, m,
					[]byte(fmt.Sprintf(`<a href="/user/%s">%s</a>`, m[1:], m)), -1)
			}
		}

		buf.Write(line)
		buf.Write(lineBreak)
	}

	rawBytes = buf.Bytes()
	ms := commitPattern.FindAll(rawBytes, -1)
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
	ms = issueIndexPattern.FindAll(rawBytes, -1)
	for _, m := range ms {
		rawBytes = bytes.Replace(rawBytes, m, []byte(fmt.Sprintf(
			`<a href="%s/issues/%s">%s</a>`, urlPrefix, m[1:], m)), -1)
	}
	return rawBytes
}

func RenderRawMarkdown(body []byte, urlPrefix string) []byte {
	htmlFlags := 0
	// htmlFlags |= gfm.HTML_USE_XHTML
	// htmlFlags |= gfm.HTML_USE_SMARTYPANTS
	// htmlFlags |= gfm.HTML_SMARTYPANTS_FRACTIONS
	// htmlFlags |= gfm.HTML_SMARTYPANTS_LATEX_DASHES
	// htmlFlags |= gfm.HTML_SKIP_HTML
	htmlFlags |= gfm.HTML_SKIP_STYLE
	htmlFlags |= gfm.HTML_SKIP_SCRIPT
	htmlFlags |= gfm.HTML_GITHUB_BLOCKCODE
	htmlFlags |= gfm.HTML_OMIT_CONTENTS
	// htmlFlags |= gfm.HTML_COMPLETE_PAGE
	renderer := &CustomRender{
		Renderer:  gfm.HtmlRenderer(htmlFlags, "", ""),
		urlPrefix: urlPrefix,
	}

	// set up the parser
	extensions := 0
	extensions |= gfm.EXTENSION_NO_INTRA_EMPHASIS
	extensions |= gfm.EXTENSION_TABLES
	extensions |= gfm.EXTENSION_FENCED_CODE
	extensions |= gfm.EXTENSION_AUTOLINK
	extensions |= gfm.EXTENSION_STRIKETHROUGH
	extensions |= gfm.EXTENSION_HARD_LINE_BREAK
	extensions |= gfm.EXTENSION_SPACE_HEADERS
	extensions |= gfm.EXTENSION_NO_EMPTY_LINE_BEFORE_BLOCK

	body = gfm.Markdown(body, renderer, extensions)
	return body
}

func RenderMarkdown(rawBytes []byte, urlPrefix string) []byte {
	body := RenderSpecialLink(rawBytes, urlPrefix)
	body = RenderRawMarkdown(body, urlPrefix)
	return body
}

func RenderMarkdownString(raw, urlPrefix string) string {
	return string(RenderMarkdown([]byte(raw), urlPrefix))
}
