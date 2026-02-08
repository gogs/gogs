package markup_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
			want:  `<a href="http://localhost:3000/user/repo/issues/3333">#3333</a>`,
		},
		{
			name:  "non-matching issue-like URL",
			input: "http://1111/2222/ssss-issues/3333?param=blah&blahh=333",
			want:  `<a href="http://1111/2222/ssss-issues/3333?param=blah&amp;blahh=333">http://1111/2222/ssss-issues/3333?param=blah&amp;blahh=333</a>`,
		},
		{
			name:  "external issue URL",
			input: "http://test.com/issues/33333",
			want:  `http://test.com/issues/33333`,
		},
		{
			name:  "commit URL from same instance",
			input: "http://localhost:3000/user/project/commit/d8a994ef243349f321568f9e36d5c3f444b99cae",
			want:  `<code><a href="http://localhost:3000/user/project/commit/d8a994ef243349f321568f9e36d5c3f444b99cae">d8a994ef24</a></code>`,
		},
		{
			name:  "commit URL with fragment from same instance",
			input: "http://localhost:3000/user/project/commit/d8a994ef243349f321568f9e36d5c3f444b99cae#diff-2",
			want:  `<code><a href="http://localhost:3000/user/project/commit/d8a994ef243349f321568f9e36d5c3f444b99cae#diff-2">d8a994ef24</a></code>`,
		},
		{
			name:  "external commit URL",
			input: "https://external-link.gogs.io/gogs/gogs/commit/d8a994ef243349f321568f9e36d5c3f444b99cae#diff-2",
			want:  `https://external-link.gogs.io/gogs/gogs/commit/d8a994ef243349f321568f9e36d5c3f444b99cae#diff-2`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := string(RawMarkdown([]byte(test.input), ""))
			require.NotEmpty(t, result)
			assert.Contains(t, result, test.want)
		})
	}
}
