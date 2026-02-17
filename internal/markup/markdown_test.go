package markup_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/conf"
	. "gogs.io/gogs/internal/markup"
)

func Test_IsMarkdownFile(t *testing.T) {
	oldExts := conf.Markdown.FileExtensions
	defer func() { conf.Markdown.FileExtensions = oldExts }()

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

func Test_RawMarkdown_AutoLink(t *testing.T) {
	oldURL := conf.Server.ExternalURL
	defer func() { conf.Server.ExternalURL = oldURL }()

	conf.Server.ExternalURL = "http://localhost:3000/"

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "issue URL from same instance",
			input: "http://localhost:3000/user/repo/issues/3333",
			want:  "<p><a href=\"http://localhost:3000/user/repo/issues/3333\">#3333</a></p>\n",
		},
		{
			name:  "non-matching issue-like URL",
			input: "http://1111/2222/ssss-issues/3333?param=blah&blahh=333",
			want:  "<p><a href=\"http://1111/2222/ssss-issues/3333?param=blah&amp;blahh=333\">http://1111/2222/ssss-issues/3333?param=blah&amp;blahh=333</a></p>\n",
		},
		{
			name:  "external issue URL",
			input: "http://test.com/issues/33333",
			want:  "<p><a href=\"http://test.com/issues/33333\">http://test.com/issues/33333</a></p>\n",
		},
		{
			name:  "commit URL from same instance",
			input: "http://localhost:3000/user/project/commit/d8a994ef243349f321568f9e36d5c3f444b99cae",
			want:  "<p> <code><a href=\"http://localhost:3000/user/project/commit/d8a994ef243349f321568f9e36d5c3f444b99cae\">d8a994ef24</a></code></p>\n",
		},
		{
			name:  "commit URL with fragment from same instance",
			input: "http://localhost:3000/user/project/commit/d8a994ef243349f321568f9e36d5c3f444b99cae#diff-2",
			want:  "<p> <code><a href=\"http://localhost:3000/user/project/commit/d8a994ef243349f321568f9e36d5c3f444b99cae#diff-2\">d8a994ef24</a></code></p>\n",
		},
		{
			name:  "external commit URL",
			input: "https://external-link.gogs.io/gogs/gogs/commit/d8a994ef243349f321568f9e36d5c3f444b99cae#diff-2",
			want:  "<p><a href=\"https://external-link.gogs.io/gogs/gogs/commit/d8a994ef243349f321568f9e36d5c3f444b99cae#diff-2\">https://external-link.gogs.io/gogs/gogs/commit/d8a994ef243349f321568f9e36d5c3f444b99cae#diff-2</a></p>\n",
		},
		{
			name:  "issue URL with single digit",
			input: "http://test.com/issues/3",
			want:  "<p><a href=\"http://test.com/issues/3\">http://test.com/issues/3</a></p>\n",
		},
		{
			name:  "host without dot in issue-like URL",
			input: "http://issues/333",
			want:  "<p><a href=\"http://issues/333\">http://issues/333</a></p>\n",
		},
		{
			name:  "https host without dot in issue-like URL",
			input: "https://issues/333",
			want:  "<p><a href=\"https://issues/333\">https://issues/333</a></p>\n",
		},
		{
			name:  "host without dot resembling keyword",
			input: "http://tissues/0",
			want:  "<p><a href=\"http://tissues/0\">http://tissues/0</a></p>\n",
		},
		{
			name:  "https commit-like URL without dot",
			input: "https://commit/d8a994ef243349f321568f9e36d5c3f444b99cae",
			want:  "<p><a href=\"https://commit/d8a994ef243349f321568f9e36d5c3f444b99cae\">https://commit/d8a994ef243349f321568f9e36d5c3f444b99cae</a></p>\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := string(RawMarkdown([]byte(test.input), ""))
			assert.Equal(t, test.want, got)
		})
	}

	t.Run("cross-repo issue URL from same instance", func(t *testing.T) {
		got := string(RawMarkdown([]byte("http://localhost:3000/other/repo/issues/42"), "/user/myrepo"))
		assert.Equal(t, "<p><a href=\"http://localhost:3000/other/repo/issues/42\">other/repo#42</a></p>\n", got)
	})

	t.Run("same-repo issue URL with fragment", func(t *testing.T) {
		got := string(RawMarkdown([]byte("http://localhost:3000/user/myrepo/issues/42#issuecomment-1"), "/user/myrepo"))
		assert.Equal(t, "<p><a href=\"http://localhost:3000/user/myrepo/issues/42#issuecomment-1\">#42</a></p>\n", got)
	})
}

func Test_RawMarkdown_LinkRewriting(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		urlPrefix string
		want      string
	}{
		{
			name:      "relative link with path-only prefix",
			input:     "[text](other-file.md)",
			urlPrefix: "/user/repo/src/branch/main",
			want:      "<p><a href=\"/user/repo/src/branch/main/other-file.md\">text</a></p>\n",
		},
		{
			name:      "relative link with absolute URL prefix",
			input:     "[text](other-file.md)",
			urlPrefix: "http://localhost:3000/user/repo/src/branch/main",
			want:      "<p><a href=\"http://localhost:3000/user/repo/src/branch/main/other-file.md\">text</a></p>\n",
		},
		{
			name:      "absolute link not rewritten",
			input:     "[text](https://example.com/page)",
			urlPrefix: "/user/repo/src/branch/main",
			want:      "<p><a href=\"https://example.com/page\">text</a></p>\n",
		},
		{
			name:      "anchor-only link not rewritten",
			input:     "[text](#section)",
			urlPrefix: "/user/repo/src/branch/main",
			want:      "<p><a href=\"#section\">text</a></p>\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := string(RawMarkdown([]byte(test.input), test.urlPrefix))
			assert.Equal(t, test.want, got)
		})
	}
}

func Test_RawMarkdown_HTMLPassthrough(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "inline HTML tags are stripped",
			input: "Hello <em>world</em>",
			want:  "<p>Hello <!-- raw HTML omitted -->world<!-- raw HTML omitted --></p>\n",
		},
		{
			name:  "block HTML tags are stripped",
			input: "<div>content</div>",
			want:  "<!-- raw HTML omitted -->\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := string(RawMarkdown([]byte(test.input), ""))
			assert.Equal(t, test.want, got)
		})
	}
}
