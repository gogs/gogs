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

func Test_MarkdownAlert(t *testing.T) {
	NewSanitizer()

	alertTitle := func(typ, icon, title string) string {
		return `<div class="markdown-alert markdown-alert-` + typ + `">` + "\n" +
			`<p class="markdown-alert-title"><span class="octicon octicon-` + icon + `" aria-hidden="true"></span>` + title + `</p>` + "\n"
	}

	tests := []struct {
		name   string
		input  string
		expVal string
	}{
		{
			name:   "note",
			input:  "> [!NOTE]\n> Useful information.\n",
			expVal: alertTitle("note", "info", "Note") + "<p>Useful information.</p>\n</div>\n",
		},
		{
			name:   "tip",
			input:  "> [!TIP]\n> A helpful suggestion.\n",
			expVal: alertTitle("tip", "light-bulb", "Tip") + "<p>A helpful suggestion.</p>\n</div>\n",
		},
		{
			name:   "important",
			input:  "> [!IMPORTANT]\n> Do not miss this.\n",
			expVal: alertTitle("important", "megaphone", "Important") + "<p>Do not miss this.</p>\n</div>\n",
		},
		{
			name:   "warning",
			input:  "> [!WARNING]\n> Watch out.\n",
			expVal: alertTitle("warning", "alert", "Warning") + "<p>Watch out.</p>\n</div>\n",
		},
		{
			name:   "caution",
			input:  "> [!CAUTION]\n> Potential danger.\n",
			expVal: alertTitle("caution", "stop", "Caution") + "<p>Potential danger.</p>\n</div>\n",
		},
		{
			name:   "marker is case-insensitive",
			input:  "> [!tip]\n> A helpful suggestion.\n",
			expVal: alertTitle("tip", "light-bulb", "Tip") + "<p>A helpful suggestion.</p>\n</div>\n",
		},
		{
			name:   "consecutive alerts",
			input:  "> [!NOTE]\n> First.\n\n> [!TIP]\n> Second.\n",
			expVal: alertTitle("note", "info", "Note") + "<p>First.</p>\n\n</div>\n" + alertTitle("tip", "light-bulb", "Tip") + "<p>Second.</p>\n</div>\n",
		},
		{
			name:   "block content",
			input:  "> [!NOTE]\n> - a\n> - b\n",
			expVal: alertTitle("note", "info", "Note") + "\n\n<ul>\n<li>a</li>\n<li>b</li>\n</ul>\n</div>\n",
		},
		{
			name:   "marker without content",
			input:  "> [!NOTE]\n",
			expVal: alertTitle("note", "info", "Note") + "\n</div>\n",
		},
		{
			name:   "block quote followed by an alert",
			input:  "> Regular quote.\n\n> [!NOTE]\n> Useful information.\n",
			expVal: "<blockquote>\n<p>Regular quote.</p>\n\n</blockquote>\n" + alertTitle("note", "info", "Note") + "<p>Useful information.</p>\n</div>\n",
		},
		{
			name:   "marker not on its own line",
			input:  "> [!NOTE] on the same line\n",
			expVal: "<blockquote>\n<p>[!NOTE] on the same line</p>\n</blockquote>\n",
		},
		{
			name:   "unknown alert type",
			input:  "> [!BOGUS]\n> Unknown type.\n",
			expVal: "<blockquote>\n<p>[!BOGUS]\nUnknown type.</p>\n</blockquote>\n",
		},
		{
			name:   "regular block quote",
			input:  "> Regular quote.\n",
			expVal: "<blockquote>\n<p>Regular quote.</p>\n</blockquote>\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expVal, string(Markdown(test.input, "/user/repo", nil)))
		})
	}
}
