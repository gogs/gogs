// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package base

import (
	"bytes"
	"path"

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

type CustomRender struct {
	gfm.Renderer
	urlPrefix string
}

func (options *CustomRender) Link(out *bytes.Buffer, link []byte, title []byte, content []byte) {
	if len(link) > 0 && !isLink(link) {
		if link[0] == '#' {
			link = append([]byte(options.urlPrefix), link...)
		} else {
			link = []byte(path.Join(options.urlPrefix, string(link)))
		}
	}

	options.Renderer.Link(out, link, title, content)
}

func RenderMarkdown(rawBytes []byte, urlPrefix string) []byte {
	htmlFlags := 0
	htmlFlags |= gfm.HTML_USE_XHTML
	// htmlFlags |= gfm.HTML_USE_SMARTYPANTS
	// htmlFlags |= gfm.HTML_SMARTYPANTS_FRACTIONS
	// htmlFlags |= gfm.HTML_SMARTYPANTS_LATEX_DASHES
	htmlFlags |= gfm.HTML_SKIP_HTML
	htmlFlags |= gfm.HTML_SKIP_STYLE
	htmlFlags |= gfm.HTML_SKIP_SCRIPT
	htmlFlags |= gfm.HTML_GITHUB_BLOCKCODE
	htmlFlags |= gfm.HTML_OMIT_CONTENTS
	htmlFlags |= gfm.HTML_COMPLETE_PAGE
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

	body := gfm.Markdown(rawBytes, renderer, extensions)

	return body
}
