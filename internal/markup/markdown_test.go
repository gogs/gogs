package markup_test

import (
	"strings"
	"testing"

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
		{input: "http://localhost:3000/user/project/commit/d8a994ef243349f321568f9e36d5c3f444b99cae", expVal: "<code><a href=\"http://localhost:3000/user/project/commit/d8a994ef243349f321568f9e36d5c3f444b99cae\">d8a994ef24</a></code>"},
		{input: "http://localhost:3000/user/project/commit/d8a994ef243349f321568f9e36d5c3f444b99cae#diff-2", expVal: "<code><a href=\"http://localhost:3000/user/project/commit/d8a994ef243349f321568f9e36d5c3f444b99cae#diff-2\">d8a994ef24</a></code>"},
		{input: "https://external-link.gogs.io/gogs/gogs/commit/d8a994ef243349f321568f9e36d5c3f444b99cae#diff-2", expVal: "<a href=\"https://external-link.gogs.io/gogs/gogs/commit/d8a994ef243349f321568f9e36d5c3f444b99cae#diff-2\">https://external-link.gogs.io/gogs/gogs/commit/d8a994ef243349f321568f9e36d5c3f444b99cae#diff-2</a>"},
		{input: "https://commit/d8a994ef243349f321568f9e36d5c3f444b99cae", expVal: "<a href=\"https://commit/d8a994ef243349f321568f9e36d5c3f444b99cae\">https://commit/d8a994ef243349f321568f9e36d5c3f444b99cae</a>"},

		// URL boundaries
		{input: "请看http://example.com/a。", expVal: "请看<a href=\"http://example.com/a\">http://example.com/a</a>。"},
		{input: "see:https://example.com/a", expVal: "see:<a href=\"https://example.com/a\">https://example.com/a</a>"},
		{input: "xhttp://example.com/a", expVal: "xhttp://example.com/a"},
		{input: "<a href=\"https://example.com/a\">https://example.com/b</a>", expVal: "<a href=\"https://example.com/a\">https://example.com/b</a>"},
		{input: "<a href=\"https://example.com/a\">见https://example.com/b</a> https://example.com/c", expVal: "<a href=\"https://example.com/a\">见https://example.com/b</a> <a href=\"https://example.com/c\">https://example.com/c</a>"},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, "<p>"+test.expVal+"</p>\n", string(RawMarkdown([]byte(test.input), "")))
		})
	}
}

func Test_RawMarkdown_RelativeLinks(t *testing.T) {
	tests := []struct {
		prefix string
		input  string
		expVal string
	}{
		{input: "[a](docs/README.md)", expVal: `<a href="/user/repo/src/main/docs/README.md">a</a>`},
		{input: "[a](../README.md)", expVal: `<a href="/user/repo/src/README.md">a</a>`},
		{input: "[a](#section)", expVal: `<a href="#section">a</a>`},
		{input: "[a](https://example.com/docs)", expVal: `<a href="https://example.com/docs">a</a>`},
		{input: "[a](//example.com/docs)", expVal: `<a href="//example.com/docs">a</a>`},

		// Emails use the absolute URL of the repository as the prefix.
		{prefix: "http://localhost:3000/user/repo", input: "[a](docs/README.md)", expVal: `<a href="http://localhost:3000/user/repo/docs/README.md">a</a>`},
		{prefix: "http://localhost:3000/user/repo", input: "[a](../../../x)", expVal: `<a href="http://localhost:3000/x">a</a>`},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			prefix := test.prefix
			if prefix == "" {
				prefix = "/user/repo/src/main"
			}
			assert.Equal(t, "<p>"+test.expVal+"</p>\n", string(RawMarkdown([]byte(test.input), prefix)))
		})
	}
}
