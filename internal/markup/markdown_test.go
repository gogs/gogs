// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package markup_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/russross/blackfriday"
	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/conf"
	. "gogs.io/gogs/internal/markup"
)

func Test_IsMarkdownFile(t *testing.T) {
	// TODO: Refactor to accept a list of extensions
	conf.Markdown.FileExtensions = strings.Split(".md,.markdown,.mdown,.mkd", ",")
	tests := []struct {
		ext    string
		expVal bool
	}{
		{ext: ".md", expVal: true},
		{ext: ".markdown", expVal: true},
		{ext: ".mdown", expVal: true},
		{ext: ".mkd", expVal: true},
		{ext: ".org", expVal: false},
		{ext: ".rst", expVal: false},
		{ext: ".asciidoc", expVal: false},
	}
	for _, test := range tests {
		assert.Equal(t, test.expVal, IsMarkdownFile(test.ext))
	}
}

func Test_Markdown(t *testing.T) {
	// TODO: Refactor to accept URL
	conf.Server.ExternalURL = "http://localhost:3000/"

	htmlFlags := 0
	htmlFlags |= blackfriday.HTML_SKIP_STYLE
	htmlFlags |= blackfriday.HTML_OMIT_CONTENTS
	renderer := &MarkdownRenderer{
		Renderer: blackfriday.HtmlRenderer(htmlFlags, "", ""),
	}

	tests := []struct {
		input  string
		expVal string
	}{
		// Issue URL
		{input: "http://localhost:3000/user/repo/issues/3333", expVal: "<a href=\"http://localhost:3000/user/repo/issues/3333\">#3333</a>"},
		{input: "http://1111/2222/ssss-issues/3333?param=blah&blahh=333", expVal: "<a href=\"http://1111/2222/ssss-issues/3333?param=blah&amp;blahh=333\">http://1111/2222/ssss-issues/3333?param=blah&amp;blahh=333</a>"},
		{input: "http://test.com/issues/33333", expVal: "<a href=\"http://test.com/issues/33333\">http://test.com/issues/33333</a>"},
		{input: "http://test.com/issues/3", expVal: "<a href=\"http://test.com/issues/3\">http://test.com/issues/3</a>"},
		{input: "http://issues/333", expVal: "<a href=\"http://issues/333\">http://issues/333</a>"},
		{input: "https://issues/333", expVal: "<a href=\"https://issues/333\">https://issues/333</a>"},
		{input: "http://tissues/0", expVal: "<a href=\"http://tissues/0\">http://tissues/0</a>"},

		// Commit URL
		{input: "http://localhost:3000/user/project/commit/d8a994ef243349f321568f9e36d5c3f444b99cae", expVal: " <code><a href=\"http://localhost:3000/user/project/commit/d8a994ef243349f321568f9e36d5c3f444b99cae\">d8a994ef24</a></code>"},
		{input: "http://localhost:3000/user/project/commit/d8a994ef243349f321568f9e36d5c3f444b99cae#diff-2", expVal: " <code><a href=\"http://localhost:3000/user/project/commit/d8a994ef243349f321568f9e36d5c3f444b99cae#diff-2\">d8a994ef24</a></code>"},
		{input: "https://external-link.gogs.io/gogs/gogs/commit/d8a994ef243349f321568f9e36d5c3f444b99cae#diff-2", expVal: "<a href=\"https://external-link.gogs.io/gogs/gogs/commit/d8a994ef243349f321568f9e36d5c3f444b99cae#diff-2\">https://external-link.gogs.io/gogs/gogs/commit/d8a994ef243349f321568f9e36d5c3f444b99cae#diff-2</a>"},
		{input: "https://commit/d8a994ef243349f321568f9e36d5c3f444b99cae", expVal: "<a href=\"https://commit/d8a994ef243349f321568f9e36d5c3f444b99cae\">https://commit/d8a994ef243349f321568f9e36d5c3f444b99cae</a>"},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			buf := new(bytes.Buffer)
			renderer.AutoLink(buf, []byte(test.input), blackfriday.LINK_TYPE_NORMAL)
			assert.Equal(t, test.expVal, buf.String())
		})
	}
}
