// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package markup_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/russross/blackfriday"
	. "github.com/smartystreets/goconvey/convey"

	. "github.com/gogits/gogs/pkg/markup"
	"github.com/gogits/gogs/pkg/setting"
)

func Test_IsMarkdownFile(t *testing.T) {
	setting.Markdown.FileExtensions = strings.Split(".md,.markdown,.mdown,.mkd", ",")
	Convey("Detect Markdown file extension", t, func() {
		testCases := []struct {
			ext   string
			match bool
		}{
			{".md", true},
			{".markdown", true},
			{".mdown", true},
			{".mkd", true},
			{".org", false},
			{".rst", false},
			{".asciidoc", false},
		}

		for _, tc := range testCases {
			So(IsMarkdownFile(tc.ext), ShouldEqual, tc.match)
		}
	})
}

func Test_Markdown(t *testing.T) {
	Convey("Rendering an issue URL", t, func() {
		setting.AppURL = "http://localhost:3000/"
		htmlFlags := 0
		htmlFlags |= blackfriday.HTML_SKIP_STYLE
		htmlFlags |= blackfriday.HTML_OMIT_CONTENTS
		renderer := &MarkdownRenderer{
			Renderer: blackfriday.HtmlRenderer(htmlFlags, "", ""),
		}
		buffer := new(bytes.Buffer)
		Convey("To the internal issue tracker", func() {
			Convey("It should render valid issue URLs", func() {
				testCases := []string{
					"http://localhost:3000/user/repo/issues/3333", "<a href=\"http://localhost:3000/user/repo/issues/3333\">#3333</a>",
				}

				for i := 0; i < len(testCases); i += 2 {
					renderer.AutoLink(buffer, []byte(testCases[i]), blackfriday.LINK_TYPE_NORMAL)

					line, _ := buffer.ReadString(0)
					So(line, ShouldEqual, testCases[i+1])
				}
			})
			Convey("It should render but not change non-issue URLs", func() {
				testCases := []string{
					"http://1111/2222/ssss-issues/3333?param=blah&blahh=333", "<a href=\"http://1111/2222/ssss-issues/3333?param=blah&amp;blahh=333\">http://1111/2222/ssss-issues/3333?param=blah&amp;blahh=333</a>",
					"http://test.com/issues/33333", "<a href=\"http://test.com/issues/33333\">http://test.com/issues/33333</a>",
					"http://test.com/issues/3", "<a href=\"http://test.com/issues/3\">http://test.com/issues/3</a>",
					"http://issues/333", "<a href=\"http://issues/333\">http://issues/333</a>",
					"https://issues/333", "<a href=\"https://issues/333\">https://issues/333</a>",
					"http://tissues/0", "<a href=\"http://tissues/0\">http://tissues/0</a>",
				}

				for i := 0; i < len(testCases); i += 2 {
					renderer.AutoLink(buffer, []byte(testCases[i]), blackfriday.LINK_TYPE_NORMAL)

					line, _ := buffer.ReadString(0)
					So(line, ShouldEqual, testCases[i+1])
				}
			})
		})
	})

	Convey("Rendering a commit URL", t, func() {
		setting.AppURL = "http://localhost:3000/"
		htmlFlags := 0
		htmlFlags |= blackfriday.HTML_SKIP_STYLE
		htmlFlags |= blackfriday.HTML_OMIT_CONTENTS
		renderer := &MarkdownRenderer{
			Renderer: blackfriday.HtmlRenderer(htmlFlags, "", ""),
		}
		buffer := new(bytes.Buffer)
		Convey("To the internal issue tracker", func() {
			Convey("It should correctly convert URLs", func() {
				testCases := []string{
					"http://localhost:3000/user/project/commit/d8a994ef243349f321568f9e36d5c3f444b99cae", " <code><a href=\"http://localhost:3000/user/project/commit/d8a994ef243349f321568f9e36d5c3f444b99cae\">d8a994ef24</a></code>",
					"http://localhost:3000/user/project/commit/d8a994ef243349f321568f9e36d5c3f444b99cae#diff-2", " <code><a href=\"http://localhost:3000/user/project/commit/d8a994ef243349f321568f9e36d5c3f444b99cae#diff-2\">d8a994ef24</a></code>",
					"https://external-link.gogs.io/gogs/gogs/commit/d8a994ef243349f321568f9e36d5c3f444b99cae#diff-2", "<a href=\"https://external-link.gogs.io/gogs/gogs/commit/d8a994ef243349f321568f9e36d5c3f444b99cae#diff-2\">https://external-link.gogs.io/gogs/gogs/commit/d8a994ef243349f321568f9e36d5c3f444b99cae#diff-2</a>",
					"https://commit/d8a994ef243349f321568f9e36d5c3f444b99cae", "<a href=\"https://commit/d8a994ef243349f321568f9e36d5c3f444b99cae\">https://commit/d8a994ef243349f321568f9e36d5c3f444b99cae</a>",
				}

				for i := 0; i < len(testCases); i += 2 {
					renderer.AutoLink(buffer, []byte(testCases[i]), blackfriday.LINK_TYPE_NORMAL)

					line, _ := buffer.ReadString(0)
					So(line, ShouldEqual, testCases[i+1])
				}
			})
		})
	})
}
