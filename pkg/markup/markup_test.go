// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package markup_test

import (
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	. "github.com/gogits/gogs/pkg/markup"
	"github.com/gogits/gogs/pkg/setting"
)

func Test_IsReadmeFile(t *testing.T) {
	Convey("Detect README file extension", t, func() {
		testCases := []struct {
			ext   string
			match bool
		}{
			{"readme", true},
			{"README", true},
			{"readme.md", true},
			{"readme.markdown", true},
			{"readme.mdown", true},
			{"readme.mkd", true},
			{"readme.org", true},
			{"readme.rst", true},
			{"readme.asciidoc", true},
			{"readme_ZH", true},
		}

		for _, tc := range testCases {
			So(IsReadmeFile(tc.ext), ShouldEqual, tc.match)
		}
	})
}

func Test_FindAllMentions(t *testing.T) {
	Convey("Find all mention patterns", t, func() {
		testCases := []struct {
			content string
			matches string
		}{
			{"@Unknwon, what do you think?", "Unknwon"},
			{"@Unknwon what do you think?", "Unknwon"},
			{"Hi @Unknwon, sounds good to me", "Unknwon"},
			{"cc/ @Unknwon @User", "Unknwon,User"},
		}

		for _, tc := range testCases {
			So(strings.Join(FindAllMentions(tc.content), ","), ShouldEqual, tc.matches)
		}
	})
}

func Test_RenderIssueIndexPattern(t *testing.T) {
	Convey("Rendering an issue reference", t, func() {
		var (
			urlPrefix                   = "/prefix"
			metas     map[string]string = nil
		)
		setting.AppSubURLDepth = 0

		Convey("To the internal issue tracker", func() {
			Convey("It should not render anything when there are no mentions", func() {
				testCases := []string{
					"",
					"this is a test",
					"test 123 123 1234",
					"#",
					"# # #",
					"# 123",
					"#abcd",
					"##1234",
					"test#1234",
					"#1234test",
					" test #1234test",
				}

				for i := 0; i < len(testCases); i++ {
					So(string(RenderIssueIndexPattern([]byte(testCases[i]), urlPrefix, metas)), ShouldEqual, testCases[i])
				}
			})
			Convey("It should render freestanding mentions", func() {
				testCases := []string{
					"#1234 test", "<a href=\"/prefix/issues/1234\">#1234</a> test",
					"test #1234 issue", "test <a href=\"/prefix/issues/1234\">#1234</a> issue",
					"test issue #1234", "test issue <a href=\"/prefix/issues/1234\">#1234</a>",
					"#5 test", "<a href=\"/prefix/issues/5\">#5</a> test",
					"test #5 issue", "test <a href=\"/prefix/issues/5\">#5</a> issue",
					"test issue #5", "test issue <a href=\"/prefix/issues/5\">#5</a>",
				}

				for i := 0; i < len(testCases); i += 2 {
					So(string(RenderIssueIndexPattern([]byte(testCases[i]), urlPrefix, metas)), ShouldEqual, testCases[i+1])
				}
			})
			Convey("It should not render issue mention without leading space", func() {
				input := []byte("test#54321 issue")
				expected := "test#54321 issue"
				So(string(RenderIssueIndexPattern(input, urlPrefix, metas)), ShouldEqual, expected)
			})
			Convey("It should not render issue mention without trailing space", func() {
				input := []byte("test #54321issue")
				expected := "test #54321issue"
				So(string(RenderIssueIndexPattern(input, urlPrefix, metas)), ShouldEqual, expected)
			})
			Convey("It should render issue mention in parentheses", func() {
				testCases := []string{
					"(#54321 issue)", "(<a href=\"/prefix/issues/54321\">#54321</a> issue)",
					"test (#54321) issue", "test (<a href=\"/prefix/issues/54321\">#54321</a>) issue",
					"test (#54321 extra) issue", "test (<a href=\"/prefix/issues/54321\">#54321</a> extra) issue",
					"test (#54321 issue)", "test (<a href=\"/prefix/issues/54321\">#54321</a> issue)",
					"test (#54321)", "test (<a href=\"/prefix/issues/54321\">#54321</a>)",
				}

				for i := 0; i < len(testCases); i += 2 {
					So(string(RenderIssueIndexPattern([]byte(testCases[i]), urlPrefix, metas)), ShouldEqual, testCases[i+1])
				}
			})
			Convey("It should render multiple issue mentions in the same line", func() {
				testCases := []string{
					"#54321 #1243", "<a href=\"/prefix/issues/54321\">#54321</a> <a href=\"/prefix/issues/1243\">#1243</a>",
					"test #54321 #1243", "test <a href=\"/prefix/issues/54321\">#54321</a> <a href=\"/prefix/issues/1243\">#1243</a>",
					"(#54321 #1243)", "(<a href=\"/prefix/issues/54321\">#54321</a> <a href=\"/prefix/issues/1243\">#1243</a>)",
					"(#54321)(#1243)", "(<a href=\"/prefix/issues/54321\">#54321</a>)(<a href=\"/prefix/issues/1243\">#1243</a>)",
					"text #54321 test #1243 issue", "text <a href=\"/prefix/issues/54321\">#54321</a> test <a href=\"/prefix/issues/1243\">#1243</a> issue",
					"#1 (#4321) test", "<a href=\"/prefix/issues/1\">#1</a> (<a href=\"/prefix/issues/4321\">#4321</a>) test",
				}

				for i := 0; i < len(testCases); i += 2 {
					So(string(RenderIssueIndexPattern([]byte(testCases[i]), urlPrefix, metas)), ShouldEqual, testCases[i+1])
				}
			})
		})
		Convey("To an external issue tracker with numeric style", func() {
			metas = make(map[string]string)
			metas["format"] = "https://someurl.com/{user}/{repo}/{index}"
			metas["user"] = "someuser"
			metas["repo"] = "somerepo"
			metas["style"] = ISSUE_NAME_STYLE_NUMERIC

			Convey("should not render anything when there are no mentions", func() {
				testCases := []string{
					"this is a test",
					"test 123 123 1234",
					"#",
					"# # #",
					"# 123",
					"#abcd",
				}

				for i := 0; i < len(testCases); i++ {
					So(string(RenderIssueIndexPattern([]byte(testCases[i]), urlPrefix, metas)), ShouldEqual, testCases[i])
				}
			})
			Convey("It should render freestanding issue mentions", func() {
				testCases := []string{
					"#1234 test", "<a href=\"https://someurl.com/someuser/somerepo/1234\">#1234</a> test",
					"test #1234 issue", "test <a href=\"https://someurl.com/someuser/somerepo/1234\">#1234</a> issue",
					"test issue #1234", "test issue <a href=\"https://someurl.com/someuser/somerepo/1234\">#1234</a>",
					"#5 test", "<a href=\"https://someurl.com/someuser/somerepo/5\">#5</a> test",
					"test #5 issue", "test <a href=\"https://someurl.com/someuser/somerepo/5\">#5</a> issue",
					"test issue #5", "test issue <a href=\"https://someurl.com/someuser/somerepo/5\">#5</a>",
				}
				for i := 0; i < len(testCases); i += 2 {
					So(string(RenderIssueIndexPattern([]byte(testCases[i]), urlPrefix, metas)), ShouldEqual, testCases[i+1])
				}
			})
			Convey("It should not render issue mention without leading space", func() {
				input := []byte("test#54321 issue")
				expected := "test#54321 issue"
				So(string(RenderIssueIndexPattern(input, urlPrefix, metas)), ShouldEqual, expected)
			})
			Convey("It should not render issue mention without trailing space", func() {
				input := []byte("test #54321issue")
				expected := "test #54321issue"
				So(string(RenderIssueIndexPattern(input, urlPrefix, metas)), ShouldEqual, expected)
			})
			Convey("It should render issue mention in parentheses", func() {
				testCases := []string{
					"(#54321 issue)", "(<a href=\"https://someurl.com/someuser/somerepo/54321\">#54321</a> issue)",
					"test (#54321) issue", "test (<a href=\"https://someurl.com/someuser/somerepo/54321\">#54321</a>) issue",
					"test (#54321 extra) issue", "test (<a href=\"https://someurl.com/someuser/somerepo/54321\">#54321</a> extra) issue",
					"test (#54321 issue)", "test (<a href=\"https://someurl.com/someuser/somerepo/54321\">#54321</a> issue)",
					"test (#54321)", "test (<a href=\"https://someurl.com/someuser/somerepo/54321\">#54321</a>)",
				}

				for i := 0; i < len(testCases); i += 2 {
					So(string(RenderIssueIndexPattern([]byte(testCases[i]), urlPrefix, metas)), ShouldEqual, testCases[i+1])
				}
			})
			Convey("It should render multiple issue mentions in the same line", func() {
				testCases := []string{
					"#54321 #1243", "<a href=\"https://someurl.com/someuser/somerepo/54321\">#54321</a> <a href=\"https://someurl.com/someuser/somerepo/1243\">#1243</a>",
					"test #54321 #1243", "test <a href=\"https://someurl.com/someuser/somerepo/54321\">#54321</a> <a href=\"https://someurl.com/someuser/somerepo/1243\">#1243</a>",
					"(#54321 #1243)", "(<a href=\"https://someurl.com/someuser/somerepo/54321\">#54321</a> <a href=\"https://someurl.com/someuser/somerepo/1243\">#1243</a>)",
					"(#54321)(#1243)", "(<a href=\"https://someurl.com/someuser/somerepo/54321\">#54321</a>)(<a href=\"https://someurl.com/someuser/somerepo/1243\">#1243</a>)",
					"text #54321 test #1243 issue", "text <a href=\"https://someurl.com/someuser/somerepo/54321\">#54321</a> test <a href=\"https://someurl.com/someuser/somerepo/1243\">#1243</a> issue",
					"#1 (#4321) test", "<a href=\"https://someurl.com/someuser/somerepo/1\">#1</a> (<a href=\"https://someurl.com/someuser/somerepo/4321\">#4321</a>) test",
				}

				for i := 0; i < len(testCases); i += 2 {
					So(string(RenderIssueIndexPattern([]byte(testCases[i]), urlPrefix, metas)), ShouldEqual, testCases[i+1])
				}
			})
		})
		Convey("To an external issue tracker with alphanumeric style", func() {
			metas = make(map[string]string)
			metas["format"] = "https://someurl.com/{user}/{repo}/?b={index}"
			metas["user"] = "someuser"
			metas["repo"] = "somerepo"
			metas["style"] = ISSUE_NAME_STYLE_ALPHANUMERIC
			Convey("It should not render anything when there are no mentions", func() {
				testCases := []string{
					"",
					"this is a test",
					"test 123 123 1234",
					"#",
					"##1234",
					"# 123",
					"#abcd",
					"test #123",
					"abc-1234",         // issue prefix must be capital
					"ABc-1234",         // issue prefix must be _all_ capital
					"ABCDEFGHIJK-1234", // the limit is 10 characters in the prefix
					"ABC1234",          // dash is required
					"test ABC- test",   // number is required
					"test -1234 test",  // prefix is required
					"testABC-123 test", // leading space is required
					"test ABC-123test", // trailing space is required
					"ABC-0123",         // no leading zero
				}

				for i := 0; i < len(testCases); i += 2 {
					So(string(RenderIssueIndexPattern([]byte(testCases[i]), urlPrefix, metas)), ShouldEqual, testCases[i])
				}
			})
			Convey("It should render freestanding issue mention", func() {
				testCases := []string{
					"OTT-1234 test", "<a href=\"https://someurl.com/someuser/somerepo/?b=OTT-1234\">OTT-1234</a> test",
					"test T-12 issue", "test <a href=\"https://someurl.com/someuser/somerepo/?b=T-12\">T-12</a> issue",
					"test issue ABCDEFGHIJ-1234567890", "test issue <a href=\"https://someurl.com/someuser/somerepo/?b=ABCDEFGHIJ-1234567890\">ABCDEFGHIJ-1234567890</a>",
					"A-1 test", "<a href=\"https://someurl.com/someuser/somerepo/?b=A-1\">A-1</a> test",
					"test ZED-1 issue", "test <a href=\"https://someurl.com/someuser/somerepo/?b=ZED-1\">ZED-1</a> issue",
					"test issue DEED-7154", "test issue <a href=\"https://someurl.com/someuser/somerepo/?b=DEED-7154\">DEED-7154</a>",
				}
				for i := 0; i < len(testCases); i += 2 {
					So(string(RenderIssueIndexPattern([]byte(testCases[i]), urlPrefix, metas)), ShouldEqual, testCases[i+1])
				}
			})
			Convey("It should render issue mention in parentheses", func() {
				testCases := []string{
					"(ABG-124 issue)", "(<a href=\"https://someurl.com/someuser/somerepo/?b=ABG-124\">ABG-124</a> issue)",
					"test (ABG-124) issue", "test (<a href=\"https://someurl.com/someuser/somerepo/?b=ABG-124\">ABG-124</a>) issue",
					"test (ABG-124 extra) issue", "test (<a href=\"https://someurl.com/someuser/somerepo/?b=ABG-124\">ABG-124</a> extra) issue",
					"test (ABG-124 issue)", "test (<a href=\"https://someurl.com/someuser/somerepo/?b=ABG-124\">ABG-124</a> issue)",
					"test (ABG-124)", "test (<a href=\"https://someurl.com/someuser/somerepo/?b=ABG-124\">ABG-124</a>)",
				}

				for i := 0; i < len(testCases); i += 2 {
					So(string(RenderIssueIndexPattern([]byte(testCases[i]), urlPrefix, metas)), ShouldEqual, testCases[i+1])
				}
			})
			Convey("It should render multiple issue mentions in the same line", func() {
				testCases := []string{
					"ABG-124 OTT-4321", "<a href=\"https://someurl.com/someuser/somerepo/?b=ABG-124\">ABG-124</a> <a href=\"https://someurl.com/someuser/somerepo/?b=OTT-4321\">OTT-4321</a>",
					"test ABG-124 OTT-4321", "test <a href=\"https://someurl.com/someuser/somerepo/?b=ABG-124\">ABG-124</a> <a href=\"https://someurl.com/someuser/somerepo/?b=OTT-4321\">OTT-4321</a>",
					"(ABG-124 OTT-4321)", "(<a href=\"https://someurl.com/someuser/somerepo/?b=ABG-124\">ABG-124</a> <a href=\"https://someurl.com/someuser/somerepo/?b=OTT-4321\">OTT-4321</a>)",
					"(ABG-124)(OTT-4321)", "(<a href=\"https://someurl.com/someuser/somerepo/?b=ABG-124\">ABG-124</a>)(<a href=\"https://someurl.com/someuser/somerepo/?b=OTT-4321\">OTT-4321</a>)",
					"text ABG-124 test OTT-4321 issue", "text <a href=\"https://someurl.com/someuser/somerepo/?b=ABG-124\">ABG-124</a> test <a href=\"https://someurl.com/someuser/somerepo/?b=OTT-4321\">OTT-4321</a> issue",
					"A-1 (RRE-345) test", "<a href=\"https://someurl.com/someuser/somerepo/?b=A-1\">A-1</a> (<a href=\"https://someurl.com/someuser/somerepo/?b=RRE-345\">RRE-345</a>) test",
				}

				for i := 0; i < len(testCases); i += 2 {
					So(string(RenderIssueIndexPattern([]byte(testCases[i]), urlPrefix, metas)), ShouldEqual, testCases[i+1])
				}
			})
		})
	})
}
