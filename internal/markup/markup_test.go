// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package markup_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	. "gogs.io/gogs/internal/markup"
)

func Test_IsReadmeFile(t *testing.T) {
	tests := []struct {
		name   string
		expVal bool
	}{
		{name: "readme", expVal: true},
		{name: "README", expVal: true},
		{name: "readme.md", expVal: true},
		{name: "readme.markdown", expVal: true},
		{name: "readme.mdown", expVal: true},
		{name: "readme.mkd", expVal: true},
		{name: "readme.org", expVal: true},
		{name: "readme.rst", expVal: true},
		{name: "readme.asciidoc", expVal: true},
		{name: "readme_ZH", expVal: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expVal, IsReadmeFile(test.name))
		})
	}
}

func Test_FindAllMentions(t *testing.T) {
	tests := []struct {
		input      string
		expMatches []string
	}{
		{input: "@unknwon, what do you think?", expMatches: []string{"unknwon"}},
		{input: "@unknwon what do you think?", expMatches: []string{"unknwon"}},
		{input: "Hi @unknwon, sounds good to me", expMatches: []string{"unknwon"}},
		{input: "cc/ @unknwon @eddycjy", expMatches: []string{"unknwon", "eddycjy"}},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, test.expMatches, FindAllMentions(test.input))
		})
	}
}

func Test_RenderIssueIndexPattern(t *testing.T) {
	urlPrefix := "/prefix"
	t.Run("render to internal issue tracker", func(t *testing.T) {
		tests := []struct {
			input  string
			expVal string
		}{
			{input: "", expVal: ""},
			{input: "this is a test", expVal: "this is a test"},
			{input: "test 123 123 1234", expVal: "test 123 123 1234"},
			{input: "#", expVal: "#"},
			{input: "# # #", expVal: "# # #"},
			{input: "# 123", expVal: "# 123"},
			{input: "#abcd", expVal: "#abcd"},
			{input: "##1234", expVal: "##1234"},
			{input: "test#1234", expVal: "test#1234"},
			{input: "#1234test", expVal: "#1234test"},
			{input: " test #1234test", expVal: " test #1234test"},

			{input: "#1234 test", expVal: "<a href=\"/prefix/issues/1234\">#1234</a> test"},
			{input: "test #1234 issue", expVal: "test <a href=\"/prefix/issues/1234\">#1234</a> issue"},
			{input: "test issue #1234", expVal: "test issue <a href=\"/prefix/issues/1234\">#1234</a>"},
			{input: "#5 test", expVal: "<a href=\"/prefix/issues/5\">#5</a> test"},
			{input: "test #5 issue", expVal: "test <a href=\"/prefix/issues/5\">#5</a> issue"},
			{input: "test issue #5", expVal: "test issue <a href=\"/prefix/issues/5\">#5</a>"},

			{input: "(#54321 issue)", expVal: "(<a href=\"/prefix/issues/54321\">#54321</a> issue)"},
			{input: "test (#54321) issue", expVal: "test (<a href=\"/prefix/issues/54321\">#54321</a>) issue"},
			{input: "test (#54321 extra) issue", expVal: "test (<a href=\"/prefix/issues/54321\">#54321</a> extra) issue"},
			{input: "test (#54321 issue)", expVal: "test (<a href=\"/prefix/issues/54321\">#54321</a> issue)"},
			{input: "test (#54321)", expVal: "test (<a href=\"/prefix/issues/54321\">#54321</a>)"},

			{input: "[#54321 issue]", expVal: "[<a href=\"/prefix/issues/54321\">#54321</a> issue]"},
			{input: "test [#54321] issue", expVal: "test [<a href=\"/prefix/issues/54321\">#54321</a>] issue"},
			{input: "test [#54321 extra] issue", expVal: "test [<a href=\"/prefix/issues/54321\">#54321</a> extra] issue"},
			{input: "test [#54321 issue]", expVal: "test [<a href=\"/prefix/issues/54321\">#54321</a> issue]"},
			{input: "test [#54321]", expVal: "test [<a href=\"/prefix/issues/54321\">#54321</a>]"},

			{input: "#54321 #1243", expVal: "<a href=\"/prefix/issues/54321\">#54321</a> <a href=\"/prefix/issues/1243\">#1243</a>"},
			{input: "test #54321 #1243", expVal: "test <a href=\"/prefix/issues/54321\">#54321</a> <a href=\"/prefix/issues/1243\">#1243</a>"},
			{input: "(#54321 #1243)", expVal: "(<a href=\"/prefix/issues/54321\">#54321</a> <a href=\"/prefix/issues/1243\">#1243</a>)"},
			{input: "(#54321)(#1243)", expVal: "(<a href=\"/prefix/issues/54321\">#54321</a>)(<a href=\"/prefix/issues/1243\">#1243</a>)"},
			{input: "text #54321 test #1243 issue", expVal: "text <a href=\"/prefix/issues/54321\">#54321</a> test <a href=\"/prefix/issues/1243\">#1243</a> issue"},
			{input: "#1 (#4321) test", expVal: "<a href=\"/prefix/issues/1\">#1</a> (<a href=\"/prefix/issues/4321\">#4321</a>) test"},
		}
		for _, test := range tests {
			t.Run(test.input, func(t *testing.T) {
				assert.Equal(t, test.expVal, string(RenderIssueIndexPattern([]byte(test.input), urlPrefix, nil)))
			})
		}
	})

	t.Run("render to external issue tracker", func(t *testing.T) {
		t.Run("numeric style", func(t *testing.T) {
			metas := map[string]string{
				"format": "https://someurl.com/{user}/{repo}/{index}",
				"user":   "someuser",
				"repo":   "somerepo",
				"style":  IssueNameStyleNumeric,
			}

			tests := []struct {
				input  string
				expVal string
			}{
				{input: "this is a test", expVal: "this is a test"},
				{input: "test 123 123 1234", expVal: "test 123 123 1234"},
				{input: "#", expVal: "#"},
				{input: "# # #", expVal: "# # #"},
				{input: "# 123", expVal: "# 123"},
				{input: "#abcd", expVal: "#abcd"},

				{input: "#1234 test", expVal: "<a href=\"https://someurl.com/someuser/somerepo/1234\">#1234</a> test"},
				{input: "test #1234 issue", expVal: "test <a href=\"https://someurl.com/someuser/somerepo/1234\">#1234</a> issue"},
				{input: "test issue #1234", expVal: "test issue <a href=\"https://someurl.com/someuser/somerepo/1234\">#1234</a>"},
				{input: "#5 test", expVal: "<a href=\"https://someurl.com/someuser/somerepo/5\">#5</a> test"},
				{input: "test #5 issue", expVal: "test <a href=\"https://someurl.com/someuser/somerepo/5\">#5</a> issue"},
				{input: "test issue #5", expVal: "test issue <a href=\"https://someurl.com/someuser/somerepo/5\">#5</a>"},

				{input: "(#54321 issue)", expVal: "(<a href=\"https://someurl.com/someuser/somerepo/54321\">#54321</a> issue)"},
				{input: "test (#54321) issue", expVal: "test (<a href=\"https://someurl.com/someuser/somerepo/54321\">#54321</a>) issue"},
				{input: "test (#54321 extra) issue", expVal: "test (<a href=\"https://someurl.com/someuser/somerepo/54321\">#54321</a> extra) issue"},
				{input: "test (#54321 issue)", expVal: "test (<a href=\"https://someurl.com/someuser/somerepo/54321\">#54321</a> issue)"},
				{input: "test (#54321)", expVal: "test (<a href=\"https://someurl.com/someuser/somerepo/54321\">#54321</a>)"},

				{input: "#54321 #1243", expVal: "<a href=\"https://someurl.com/someuser/somerepo/54321\">#54321</a> <a href=\"https://someurl.com/someuser/somerepo/1243\">#1243</a>"},
				{input: "test #54321 #1243", expVal: "test <a href=\"https://someurl.com/someuser/somerepo/54321\">#54321</a> <a href=\"https://someurl.com/someuser/somerepo/1243\">#1243</a>"},
				{input: "(#54321 #1243)", expVal: "(<a href=\"https://someurl.com/someuser/somerepo/54321\">#54321</a> <a href=\"https://someurl.com/someuser/somerepo/1243\">#1243</a>)"},
				{input: "(#54321)(#1243)", expVal: "(<a href=\"https://someurl.com/someuser/somerepo/54321\">#54321</a>)(<a href=\"https://someurl.com/someuser/somerepo/1243\">#1243</a>)"},
				{input: "text #54321 test #1243 issue", expVal: "text <a href=\"https://someurl.com/someuser/somerepo/54321\">#54321</a> test <a href=\"https://someurl.com/someuser/somerepo/1243\">#1243</a> issue"},
				{input: "#1 (#4321) test", expVal: "<a href=\"https://someurl.com/someuser/somerepo/1\">#1</a> (<a href=\"https://someurl.com/someuser/somerepo/4321\">#4321</a>) test"},
			}
			for _, test := range tests {
				t.Run(test.input, func(t *testing.T) {
					assert.Equal(t, test.expVal, string(RenderIssueIndexPattern([]byte(test.input), urlPrefix, metas)))
				})
			}
		})

		t.Run("alphanumeric style", func(t *testing.T) {
			metas := map[string]string{
				"format": "https://someurl.com/{user}/{repo}/?b={index}",
				"user":   "someuser",
				"repo":   "somerepo",
				"style":  IssueNameStyleAlphanumeric,
			}

			tests := []struct {
				input  string
				expVal string
			}{
				{input: "", expVal: ""},
				{input: "this is a test", expVal: "this is a test"},
				{input: "test 123 123 1234", expVal: "test 123 123 1234"},
				{input: "#", expVal: "#"},
				{input: "##1234", expVal: "##1234"},
				{input: "# 123", expVal: "# 123"},
				{input: "#abcd", expVal: "#abcd"},
				{input: "test #123", expVal: "test #123"},
				{input: "abc-1234", expVal: "abc-1234"},                 // issue prefix must be capital
				{input: "ABc-1234", expVal: "ABc-1234"},                 // issue prefix must be _all_ capital
				{input: "ABCDEFGHIJK-1234", expVal: "ABCDEFGHIJK-1234"}, // the limit is 10 characters in the prefix
				{input: "ABC1234", expVal: "ABC1234"},                   // dash is required
				{input: "test ABC- test", expVal: "test ABC- test"},     // number is required
				{input: "test -1234 test", expVal: "test -1234 test"},   // prefix is required
				{input: "testABC-123 test", expVal: "testABC-123 test"}, // leading space is required
				{input: "test ABC-123test", expVal: "test ABC-123test"}, // trailing space is required
				{input: "ABC-0123", expVal: "ABC-0123"},                 // no leading zero

				{input: "OTT-1234 test", expVal: "<a href=\"https://someurl.com/someuser/somerepo/?b=OTT-1234\">OTT-1234</a> test"},
				{input: "test T-12 issue", expVal: "test <a href=\"https://someurl.com/someuser/somerepo/?b=T-12\">T-12</a> issue"},
				{input: "test issue ABCDEFGHIJ-1234567890", expVal: "test issue <a href=\"https://someurl.com/someuser/somerepo/?b=ABCDEFGHIJ-1234567890\">ABCDEFGHIJ-1234567890</a>"},
				{input: "A-1 test", expVal: "<a href=\"https://someurl.com/someuser/somerepo/?b=A-1\">A-1</a> test"},
				{input: "test ZED-1 issue", expVal: "test <a href=\"https://someurl.com/someuser/somerepo/?b=ZED-1\">ZED-1</a> issue"},
				{input: "test issue DEED-7154", expVal: "test issue <a href=\"https://someurl.com/someuser/somerepo/?b=DEED-7154\">DEED-7154</a>"},

				{input: "(ABG-124 issue)", expVal: "(<a href=\"https://someurl.com/someuser/somerepo/?b=ABG-124\">ABG-124</a> issue)"},
				{input: "test (ABG-124) issue", expVal: "test (<a href=\"https://someurl.com/someuser/somerepo/?b=ABG-124\">ABG-124</a>) issue"},
				{input: "test (ABG-124 extra) issue", expVal: "test (<a href=\"https://someurl.com/someuser/somerepo/?b=ABG-124\">ABG-124</a> extra) issue"},
				{input: "test (ABG-124 issue)", expVal: "test (<a href=\"https://someurl.com/someuser/somerepo/?b=ABG-124\">ABG-124</a> issue)"},
				{input: "test (ABG-124)", expVal: "test (<a href=\"https://someurl.com/someuser/somerepo/?b=ABG-124\">ABG-124</a>)"},

				{input: "[ABG-124] issue", expVal: "[<a href=\"https://someurl.com/someuser/somerepo/?b=ABG-124\">ABG-124</a>] issue"},
				{input: "test [ABG-124] issue", expVal: "test [<a href=\"https://someurl.com/someuser/somerepo/?b=ABG-124\">ABG-124</a>] issue"},
				{input: "test [ABG-124 extra] issue", expVal: "test [<a href=\"https://someurl.com/someuser/somerepo/?b=ABG-124\">ABG-124</a> extra] issue"},
				{input: "test [ABG-124 issue]", expVal: "test [<a href=\"https://someurl.com/someuser/somerepo/?b=ABG-124\">ABG-124</a> issue]"},
				{input: "test [ABG-124]", expVal: "test [<a href=\"https://someurl.com/someuser/somerepo/?b=ABG-124\">ABG-124</a>]"},

				{input: "ABG-124 OTT-4321", expVal: "<a href=\"https://someurl.com/someuser/somerepo/?b=ABG-124\">ABG-124</a> <a href=\"https://someurl.com/someuser/somerepo/?b=OTT-4321\">OTT-4321</a>"},
				{input: "test ABG-124 OTT-4321", expVal: "test <a href=\"https://someurl.com/someuser/somerepo/?b=ABG-124\">ABG-124</a> <a href=\"https://someurl.com/someuser/somerepo/?b=OTT-4321\">OTT-4321</a>"},
				{input: "(ABG-124 OTT-4321)", expVal: "(<a href=\"https://someurl.com/someuser/somerepo/?b=ABG-124\">ABG-124</a> <a href=\"https://someurl.com/someuser/somerepo/?b=OTT-4321\">OTT-4321</a>)"},
				{input: "(ABG-124)(OTT-4321)", expVal: "(<a href=\"https://someurl.com/someuser/somerepo/?b=ABG-124\">ABG-124</a>)(<a href=\"https://someurl.com/someuser/somerepo/?b=OTT-4321\">OTT-4321</a>)"},
				{input: "text ABG-124 test OTT-4321 issue", expVal: "text <a href=\"https://someurl.com/someuser/somerepo/?b=ABG-124\">ABG-124</a> test <a href=\"https://someurl.com/someuser/somerepo/?b=OTT-4321\">OTT-4321</a> issue"},
				{input: "A-1 (RRE-345) test", expVal: "<a href=\"https://someurl.com/someuser/somerepo/?b=A-1\">A-1</a> (<a href=\"https://someurl.com/someuser/somerepo/?b=RRE-345\">RRE-345</a>) test"},
			}
			for _, test := range tests {
				t.Run(test.input, func(t *testing.T) {
					assert.Equal(t, test.expVal, string(RenderIssueIndexPattern([]byte(test.input), urlPrefix, metas)))
				})
			}
		})
	})
}

func TestRenderSha1CurrentPattern(t *testing.T) {
	metas := map[string]string{
		"repoLink": "/someuser/somerepo",
	}

	tests := []struct {
		desc   string
		input  string
		prefix string
		expVal string
	}{
		{
			desc:   "Full SHA (40 symbols)",
			input:  "ad8ced4f57d9068cb2874557245be3c7f341149d",
			prefix: metas["repoLink"],
			expVal: `<a href="/someuser/somerepo/commit/ad8ced4f57d9068cb2874557245be3c7f341149d"><code>ad8ced4f57</code></a>`,
		},
		{
			desc:   "Short SHA (8 symbols)",
			input:  "ad8ced4f",
			prefix: metas["repoLink"],
			expVal: `<a href="/someuser/somerepo/commit/ad8ced4f"><code>ad8ced4f</code></a>`,
		},
		{
			desc:   "9 digits",
			input:  "123456789",
			prefix: metas["repoLink"],
			expVal: "123456789",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			assert.Equal(t, test.expVal, string(RenderSha1CurrentPattern([]byte(test.input), test.prefix)))
		})
	}
}
