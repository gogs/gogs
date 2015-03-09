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

	"github.com/russross/blackfriday"

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

// See http://www.w3.org/TR/html-markup/syntax.html#attribute
const HtmlAttributePattern =
  `(?:` +
    `(?P<attr_name>[^\s\x00"'>/=\p{Cc}]+)` +
    `(?:` + // optional value
      `\s*=\s*` +
      `(?P<attr_value>` +
        `"[^\x00\p{Cc}"]*"` + // double-quoted
        `'[^\x00\p{Cc}']*'` + // single-quoted
        `[^\x00\p{Cc}\s"'=<>\x60]+` +   // unquoted
      `)` +
    `)?` +
  `)`

const HtmlCommentPattern = `<!--(?:[^-]|-[^-]|--[^>])*-->`

var (
	MentionRegex      = regexp.MustCompile(`(\s|^)@[0-9a-zA-Z_]+`)
	CommitRegex       = regexp.MustCompile(`(\s|^)https?.*commit/[0-9a-zA-Z]+(#+[0-9a-zA-Z-]*)?`)
	IssueFullRegex    = regexp.MustCompile(`(\s|^)https?.*issues/[0-9]+(#+[0-9a-zA-Z-]*)?`)
	IssueIndexRegex   = regexp.MustCompile(`( |^)#[0-9]+`)
	Sha1CurrentRegex  = regexp.MustCompile(`\b[0-9a-f]{40}\b`)
	HtmlOpenTagRegex  = regexp.MustCompile(MkHtmlOpenTagPattern(`[a-zA-Z0-9]+`))
	HtmlCloseTagRegex = regexp.MustCompile(MkHtmlCloseTagPattern(`[a-zA-Z0-9]+`))
	HtmlCommentRegex  = regexp.MustCompile(HtmlCommentPattern)
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
			ms := MentionRegex.FindAll(line, -1)
			for _, m := range ms {
				m = bytes.TrimSpace(m)
				line = bytes.Replace(line, m,
					[]byte(fmt.Sprintf(`<a href="%s/%s">%s</a>`, setting.AppSubUrl, m[1:], m)), -1)
			}
		}

		buf.Write(line)
		buf.Write(lineBreak)
	}

	rawBytes = buf.Bytes()
	ms := CommitRegex.FindAll(rawBytes, -1)
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
	ms = IssueFullRegex.FindAll(rawBytes, -1)
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
	return rawBytes
}

func RenderSha1CurrentPattern(rawBytes []byte, urlPrefix string) []byte {
	ms := Sha1CurrentRegex.FindAll(rawBytes, -1)
	for _, m := range ms {
		rawBytes = bytes.Replace(rawBytes, m, []byte(fmt.Sprintf(
			`<a href="%s/commit/%s"><code>%s</code></a>`, urlPrefix, m, ShortSha(string(m)))), -1)
	}
	return rawBytes
}

func RenderIssueIndexPattern(rawBytes []byte, urlPrefix string) []byte {
	ms := IssueIndexRegex.FindAll(rawBytes, -1)
	for _, m := range ms {
		rawBytes = bytes.Replace(rawBytes, m, []byte(fmt.Sprintf(`<a href="%s/issues/%s">%s</a>`,
			urlPrefix, strings.TrimPrefix(string(m[1:]), "#"), m)), -1)
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
		Renderer:	blackfriday.HtmlRenderer(htmlFlags, "", ""),
		urlPrefix: urlPrefix,
	}

	// set up the parser
	extensions := 0
	extensions |= blackfriday.EXTENSION_NO_INTRA_EMPHASIS
	extensions |= blackfriday.EXTENSION_TABLES
	extensions |= blackfriday.EXTENSION_FENCED_CODE
	extensions |= blackfriday.EXTENSION_AUTOLINK
	extensions |= blackfriday.EXTENSION_STRIKETHROUGH
	extensions |= blackfriday.EXTENSION_HARD_LINE_BREAK
	extensions |= blackfriday.EXTENSION_SPACE_HEADERS
	extensions |= blackfriday.EXTENSION_NO_EMPTY_LINE_BEFORE_BLOCK

	body = blackfriday.Markdown(body, renderer, extensions)
	return body
}

func RenderMarkdown(rawBytes []byte, urlPrefix string) []byte {
	return PostProcessMarkdown(RenderRawMarkdown(rawBytes, urlPrefix), urlPrefix)
}

func RenderMarkdownString(raw, urlPrefix string) string {
	return string(RenderMarkdown([]byte(raw), urlPrefix))
}

func PostProcessMarkdown(html []byte, urlPrefix string) []byte {
	processed := make([]byte, len(html) + len(html) / 2)
	for i, part := range GetPostProcessableParts(html) {
		if (0 == i & 1) && len(part) > 0 {
			part = RenderSpecialLink(part, urlPrefix)
			part = RenderSha1CurrentPattern(part, urlPrefix)
			part = RenderIssueIndexPattern(part, urlPrefix)
		}

		processed = append(processed, part...)
	}

	return processed
}

// Breaks the provided HTML into processable and non-processable content, with processable content at
// even indices, and non-processable content (tags) at odd indices.
func GetPostProcessableParts(html []byte) [][]byte {
	// Strip comments from the input so that we don't have to deal with tags contained in comments
	html = HtmlCommentRegex.ReplaceAll(html, []byte{})

	openMatches	:= HtmlOpenTagRegex.FindAllSubmatchIndex(html, -1)
	closeMatches := HtmlCloseTagRegex.FindAllSubmatchIndex(html, -1)

	oi := 0; ol := len(openMatches)
	ci := 0; cl := len(closeMatches)

	parts := make([][]byte, 2 * (ol + cl) + 1) // we have fence sections
	lastOffset := 0
	for ci < cl && oi < ol {
		// Does an open tag occur next?
		if oi < ol && (ci >= cl || openMatches[oi][0] < closeMatches[ci][0]) {
			openMatch := openMatches[oi]
			openTagName := string(html[openMatch[2]:openMatch[3]])
			foundClose := false

			// If this is an excluded tag, its content is not post-processable and we need to skip to the end
			// of its matching close tag (we assume that you cannot nest tags)
			if IsExcludedTagName(openTagName) {
				for closeSearchIndex := ci; closeSearchIndex < cl; closeSearchIndex++ {
					closeMatch := closeMatches[closeSearchIndex]

					// Skip close tags occurring before the open tag
					if closeMatch[0] < openMatch[1] { continue }

					closeTagName := string(html[closeMatch[2]:closeMatch[3]])
					if strings.EqualFold(openTagName, closeTagName) {
						foundClose = true

						parts = append(parts, html[lastOffset:openMatch[0]], html[openMatch[0]:closeMatch[1]])
						lastOffset = closeMatch[1]

						ci = closeSearchIndex + 1
						// Advance the open tag index until we are at a tag occurring after the close tag
						for oi < ol && openMatches[oi][0] < closeMatch[1] { oi++ }

						break
					}
				}
			}

			// If we did not find a close tag (or this is not an excluded tag), then we just add the open tag
			// as the skipped part.
			if !foundClose {
				parts = append(parts, html[lastOffset:openMatch[0]], html[openMatch[0]:openMatch[1]])
				lastOffset = openMatch[1]

				oi += 1
				for ci < cl && closeMatches[ci][0] < openMatch[1] { ci++ }
			}
		} else {
			closeMatch := closeMatches[ci]

			parts = append(parts, html[lastOffset:closeMatch[0]], html[closeMatch[0]:closeMatch[1]])
			lastOffset = closeMatch[1]

			ci += 1
			for oi < ol && openMatches[oi][0] < closeMatch[1] { oi++ }
		}
	}

	if(lastOffset < len(html)) {
		parts = append(parts, html[lastOffset:])
	}

	return parts
}

func IsExcludedTagName(tagName string) bool {
	return strings.EqualFold("a", tagName) ||
				 strings.EqualFold("code", tagName) ||
				 strings.EqualFold("pre", tagName)
}

// Creates an uncompiled regular expression matching an HTML tag using the provided tag name pattern.
// Subpattern 1 (`tag_name`) contains the matched tag name. Subpattern 2 (`tag_attributes`) contains the
// tag's attribute string. Subpattern 3 (`tag_void`) contains the tag's void slash (if any).
func MkHtmlOpenTagPattern(tagNamePattern string) string {
  return `(?:` +
           `<(?P<tag_name>` + tagNamePattern + `)` +
           `(?P<tag_attributes>(?:\s+` + HtmlAttributePattern + `)*)` +
           `(?:\s*(?P<tag_void>/))?` +
           `\s*>` +
         `)`
}

// Creates an uncompiled regular expression matching an HTML close tag using the provided tag name pattern.
// Subpattern 1 (`tag_close`) contains the matched tag name.
func MkHtmlCloseTagPattern(tagNamePattern string) string {
	return `</(?P<tag_close>` + tagNamePattern + `)\s*>`
}
