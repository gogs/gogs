package markup

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/markup/dariomark"
)

func TestIsMarkdownFile(t *testing.T) {
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

func TestMarkdown(t *testing.T) {
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

func TestRawMarkdown_RelativeLinks(t *testing.T) {
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

func trackerMetas() map[string]string {
	return map[string]string{
		"repoLink": "/user/repo",
		"user":     "user",
		"repo":     "repo",
		"format":   "https://tracker.example.com/{user}/{repo}/issues/{index}",
		"style":    "numeric",
	}
}

func TestMarkdown_SpecialLinks(t *testing.T) {
	conf.Server.ExternalURL = "http://localhost:3000/"
	NewSanitizer()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "mentions and issues",
			input: "@user, see #12 and user/repo#7.",
			want:  "<p><a href=\"/user\" rel=\"nofollow\">@user</a>, see <a href=\"https://tracker.example.com/user/repo/issues/12\" rel=\"nofollow\">#12</a> and <a href=\"http://localhost:3000/user/repo/issues/7\" rel=\"nofollow\">user/repo#7</a>.</p>\n",
		},
		{
			name:  "links and code are left alone",
			input: "[#12](http://example.com) `#12` and ```@user```",
			want:  "<p><a href=\"http://example.com\" rel=\"nofollow\">#12</a> <code>#12</code> and <code>@user</code></p>\n",
		},
		{
			name:  "raw text elements are left alone",
			input: "<textarea>@user #12</textarea> <script>@user</script> @user",
			want:  "@user #12  <a href=\"/user\" rel=\"nofollow\">@user</a>\n",
		},
		{
			name:  "relative image",
			input: "![logo](images/logo.png)",
			want:  "<p><a href=\"/user/repo/images/logo.png\" rel=\"nofollow\"><img src=\"/user/repo/images/logo.png\" alt=\"logo\"></a></p>\n",
		},
		{
			name:  "absolute image with title",
			input: "![logo](https://example.com/logo.png \"Logo\")",
			want:  "<p><a href=\"https://example.com/logo.png\" rel=\"nofollow\"><img src=\"https://example.com/logo.png\" alt=\"logo\" title=\"Logo\"></a></p>\n",
		},
		{
			name:  "image in a link",
			input: "[![badge](badge.svg)](https://example.com)",
			want:  "<p><a href=\"https://example.com\" rel=\"nofollow\"><img src=\"/user/repo/badge.svg\" alt=\"badge\"></a></p>\n",
		},
		{
			name:  "image with a path from the root in a link",
			input: "[![badge](/img/badge.svg)](https://example.com)",
			want:  "<p><a href=\"https://example.com\" rel=\"nofollow\"><img src=\"/user/repo/img/badge.svg\" alt=\"badge\"></a></p>\n",
		},
		{
			name:  "absolute image in a link",
			input: "[![badge](https://example.com/badge.svg)](https://example.com)",
			want:  "<p><a href=\"https://example.com\" rel=\"nofollow\"><img src=\"https://example.com/badge.svg\" alt=\"badge\"></a></p>\n",
		},
		{
			name:  "image in a raw anchor",
			input: "<a href=\"https://example.com\"><img src=\"badge.svg\"></a> and <a href=\"https://example.com\"><code><img src=\"x.png\"></code></a>",
			want:  "<p><a href=\"https://example.com\" rel=\"nofollow\"><img src=\"/user/repo/badge.svg\"></a> and <a href=\"https://example.com\" rel=\"nofollow\"><code><img src=\"x.png\"></code></a></p>\n",
		},
		{
			name:  "self-closing raw images",
			input: "<a href=\"https://example.com\"><img src=\"badge.svg\" /></a> <img src=\"logo.png\" width=\"10\"/>",
			want:  "<p><a href=\"https://example.com\" rel=\"nofollow\"><img src=\"/user/repo/badge.svg\"/></a> <a href=\"/user/repo/logo.png\" rel=\"nofollow\"><img src=\"/user/repo/logo.png\" width=\"10\"/></a></p>\n",
		},
		{
			name:  "raw anchor",
			input: "<a href=\"https://example.com\">#12 @user</a> #13",
			want:  "<p><a href=\"https://example.com\" rel=\"nofollow\">#12 @user</a> <a href=\"https://tracker.example.com/user/repo/issues/13\" rel=\"nofollow\">#13</a></p>\n",
		},
		{
			name:  "raw HTML text and image",
			input: "<p>@user</p>\n\n<img src=\"images/a.png\"> <code>#12</code>",
			want:  "<p><a href=\"/user\" rel=\"nofollow\">@user</a></p>\n<p><a href=\"/user/repo/images/a.png\" rel=\"nofollow\"><img src=\"/user/repo/images/a.png\"></a> <code>#12</code></p>\n",
		},
		{
			name:  "centered raw HTML with sized relative images",
			input: "<div align=\"center\">\n<img src=\"logo.png\" width=\"100\" height=\"50%\" alt=\"logo\"> <img src=\"https://example.com/b.svg\" width=\"80\">\n</div>\n\n<p align=\"middle\"><img src=\"x.png\" width=\"10em\" align=\"right\"></p>",
			want:  "<div align=\"center\">\n<a href=\"/user/repo/logo.png\" rel=\"nofollow\"><img src=\"/user/repo/logo.png\" width=\"100\" height=\"50%\" alt=\"logo\"></a> <a href=\"https://example.com/b.svg\" rel=\"nofollow\"><img src=\"https://example.com/b.svg\" width=\"80\"></a>\n</div>\n<p><a href=\"/user/repo/x.png\" rel=\"nofollow\"><img src=\"/user/repo/x.png\" align=\"right\"></a></p>\n",
		},
		{
			name:  "raw anchor across blocks",
			input: "<a href=\"x\">\n\n#12\n\n</a> #13",
			want:  "<a href=\"x\" rel=\"nofollow\">\n<p>#12</p>\n<p></a> #13</p>\n",
		},
		{
			name:  "issue URLs on this instance",
			input: "http://localhost:3000/user/repo/issues/3 and http://localhost:3000/other/repo/issues/4",
			want:  "<p><a href=\"http://localhost:3000/user/repo/issues/3\" rel=\"nofollow\">#3</a> and <a href=\"http://localhost:3000/other/repo/issues/4\" rel=\"nofollow\">other/repo#4</a></p>\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, string(Markdown(test.input, "/user/repo", trackerMetas())))
		})
	}
}

// TestMarkdown_LineBreaks checks that Markdown other than files keeps its line
// breaks, and files keep them only with [markdown] ENABLE_HARD_LINE_BREAK.
func TestMarkdown_LineBreaks(t *testing.T) {
	NewSanitizer()
	defer func(old bool) { conf.Markdown.EnableHardLineBreak = old }(conf.Markdown.EnableHardLineBreak)

	const input = "Fixed it\n:tada: thanks"
	conf.Markdown.EnableHardLineBreak = false
	assert.Equal(t, "<p>Fixed it<br>\n🎉 thanks</p>\n", string(Markdown(input, "/user/repo", nil)))
	assert.Equal(t, "<p>Fixed it\n:tada: thanks</p>\n", string(MarkdownFile(input, "/user/repo", nil)))
	assert.Equal(t, "<p>Fixed it\n:tada: thanks</p>\n", string(RawMarkdown([]byte(input), "")))

	conf.Markdown.EnableHardLineBreak = true
	assert.Equal(t, "<p>Fixed it<br>\n🎉 thanks</p>\n", string(MarkdownFile(input, "/user/repo", nil)))
}

// TestMarkdown_Emoji checks that emoji shortcodes render as on GitHub, with
// and without raw HTML in the document. It renders files, whose line endings
// do not split text.
func TestMarkdown_Emoji(t *testing.T) {
	conf.Server.ExternalURL = "http://localhost:3000/"
	NewSanitizer()

	// Text this long is passed to SpecialLinks in pieces, here split at the
	// line ending.
	long := strings.Repeat("x", 40000)
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "known names",
			input: "Ship it :tada: :+1: :nope: :Tada: :octocat:",
			want:  "<p>Ship it 🎉 👍 :nope: :Tada: :octocat:</p>\n",
		},
		{
			name:  "first shortcode after a word char",
			input: "At 10:30:45 :tada:",
			want:  "<p>At 10:30:45 :tada:</p>\n",
		},
		{
			name:  "first shortcode after a space",
			input: ":tada: a:b:c:tada:",
			want:  "<p>🎉 a🅱️c🎉</p>\n",
		},
		{
			name:  "first shortcode after a line ending, tab or no-break space",
			input: "x\n:tada: x\t:tada:\n\nx&nbsp;:tada:",
			want:  "<p>x\n:tada: x\t:tada:</p>\n<p>x\u00a0:tada:</p>\n",
		},
		{
			name:  "closing colon opens the next shortcode",
			input: "x :zz:tada::tada: :a:b:c:",
			want:  "<p>x :zz🎉🎉 🅰️b:c:</p>\n",
		},
		{
			name:  "elements and special links split text",
			input: "x:tada: **:tada:** [x :tada:](https://example.com) `:tada:` @user:tada:",
			want:  "<p>x:tada: <strong>🎉</strong> <a href=\"https://example.com\" rel=\"nofollow\">x 🎉</a> <code>:tada:</code> <a href=\"/user\" rel=\"nofollow\">@user</a>🎉</p>\n",
		},
		{
			name:  "special link after a word char",
			input: "(#12:tada:)",
			want:  "<p>(<a href=\"/user/repo/issues/12\" rel=\"nofollow\">#12</a>🎉)</p>\n",
		},
		{
			name:  "code block",
			input: "```\n:tada:\n```",
			want:  "<pre><code>:tada:\n</code></pre>\n",
		},
		{
			name:  "heading",
			input: "# :tada: Release",
			want:  "<h1 id=\"user-content-tada-release\">🎉 Release</h1>\n",
		},
		{
			name:  "long text after a line ending",
			input: long + "\n:tada:",
			want:  "<p>" + long + "\n:tada:</p>\n",
		},
		{
			name:  "long text after a shortcode",
			input: ":tada: " + long + "\nx:tada:",
			want:  "<p>🎉 " + long + "\nx🎉</p>\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res := renderMarkdown([]byte(test.input), "/user/repo", nil, false, false)
			require.True(t, res.Sanitized, "takes the fast path")
			assert.Equal(t, test.want, string(MarkdownFile(test.input, "/user/repo", nil)))
			// A raw HTML comment, which renders as a line ending, takes the
			// path that tokenizes the HTML.
			assert.Equal(t, test.want+"\n", string(MarkdownFile(test.input+"\n\n<!-- -->", "/user/repo", nil)))
		})
	}

	rawTests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "raw elements",
			input: "<kbd>:tada:</kbd> <tt>:tada:</tt> <code>:tada:</code> <pre>:tada:</pre> <a href=\"x\">:tada: <code>:tada:</code></a>",
			want:  "<p><kbd>🎉</kbd> <tt>🎉</tt> <code>:tada:</code> <pre>:tada:</pre> <a href=\"x\" rel=\"nofollow\">🎉 <code>:tada:</code></a></p>\n",
		},
		{
			name:  "end tag without a start tag",
			input: "x</b>:tada: <span>x</b>:tada:</span>",
			want:  "<p>x</p>:tada: <span>x</span>:tada:</span></p>\n",
		},
		{
			name:  "leading whitespace",
			input: "x<b>\t:tada:</b> x<b>y\t:tada:</b>",
			want:  "<p>x<b>\t🎉</b> x<b>y\t:tada:</b></p>\n",
		},
	}
	for _, test := range rawTests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, string(MarkdownFile(test.input, "/user/repo", nil)))
		})
	}
}

func TestMarkdown_FastPathXSS(t *testing.T) {
	conf.Server.ExternalURL = "http://localhost:3000/"
	NewSanitizer()

	tests := []struct {
		input string
		want  string
	}{
		{input: "[x](javascript:alert(1))", want: "<p><a href=\"/user/repo/javascript:alert(1)\" rel=\"nofollow\">x</a></p>\n"},
		{input: "[x](JaVaScRiPt:alert(1))", want: "<p><a href=\"/user/repo/JaVaScRiPt:alert(1)\" rel=\"nofollow\">x</a></p>\n"},
		{input: "[x](java&#x09;script:alert(1))", want: "<p><a href=\"/user/repo/java%09script:alert(1)\" rel=\"nofollow\">x</a></p>\n"},
		{input: "[x](&#106;avascript:alert(1))", want: "<p><a href=\"/user/repo/javascript:alert(1)\" rel=\"nofollow\">x</a></p>\n"},
		{input: "[x](javascript&#58;alert(1))", want: "<p><a href=\"/user/repo/javascript:alert(1)\" rel=\"nofollow\">x</a></p>\n"},
		{input: "[x](javascript://alert(1))", want: "<p>x</p>\n"},
		{input: "[x](JaVaScRiPt://%0Aalert(1))", want: "<p><a href=\"/user/repo/JaVaScRiPt:/%0Aalert(1)\" rel=\"nofollow\">x</a></p>\n"},
		{input: "<javascript:alert(1)>", want: "<p>javascript:alert(1)</p>\n"},
		{input: "<vbscript:msgbox(1)>", want: "<p>vbscript:msgbox(1)</p>\n"},
		{input: "[x]\n\n[x]: vbscript:msgbox(1)", want: "<p><a href=\"/user/repo/vbscript:msgbox(1)\" rel=\"nofollow\">x</a></p>\n"},
		{input: "![x](data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==)", want: "<p><img alt=\"x\"></p>\n"},
		{input: "![x](data:image/svg+xml,<svg/onload=alert(1)>)", want: "<p><img alt=\"x\"></p>\n"},
		{input: "![x](data:image/png;base64,iVBORw0KGgo=)", want: "<p><img src=\"data:image/png;base64,iVBORw0KGgo=\" alt=\"x\"></p>\n"},
		{input: "[x](//evil.example.com/a)", want: "<p><a href=\"//evil.example.com/a\" rel=\"nofollow\">x</a></p>\n"},
		{input: "[x](\\\\evil.example.com)", want: "<p><a href=\"/user/repo/%5Cevil.example.com\" rel=\"nofollow\">x</a></p>\n"},
		{input: "[x](%6Aavascript:alert(1))", want: "<p><a href=\"/user/repo/%6Aavascript:alert(1)\" rel=\"nofollow\">x</a></p>\n"},
		{input: "[x](https://example.com/\"onmouseover=\"alert(1))", want: "<p><a href=\"https://example.com/%22onmouseover=%22alert(1)\" rel=\"nofollow\">x</a></p>\n"},
		{input: "[x](https://example.com \"t\\\" onclick=\\\"alert(1)\")", want: "<p><a href=\"https://example.com\" rel=\"nofollow\">x</a></p>\n"},
		{input: "![a](i.png \"<t>\")", want: "<p><a href=\"/user/repo/i.png\" rel=\"nofollow\"><img src=\"/user/repo/i.png\" alt=\"a\"></a></p>\n"},
		{input: "[a&b](https://example.com/?a=1&b=<2> \"x & y\")", want: "<p><a href=\"https://example.com/?a=1&amp;b=%3C2%3E\" rel=\"nofollow\">a&amp;b</a></p>\n"},
		{input: "```js\" onclick=\"alert(1)\nx\n```", want: "<pre><code>x\n</code></pre>\n"},
		{input: "```c++\nx\n```", want: "<pre><code class=\"language-c++\">x\n</code></pre>\n"},
		{input: "&lt;script&gt;alert(1)&lt;/script&gt; \\<script>", want: "<p>&lt;script&gt;alert(1)&lt;/script&gt; &lt;script&gt;</p>\n"},
		{input: "`<script>alert(1)</script>`", want: "<p><code>&lt;script&gt;alert(1)&lt;/script&gt;</code></p>\n"},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			res := renderMarkdown([]byte(test.input), "/user/repo", trackerMetas(), true, false)
			assert.True(t, res.Sanitized, "takes the fast path")
			assert.Equal(t, test.want, string(Markdown(test.input, "/user/repo", trackerMetas())))
		})
	}
}

// TestMarkdown_ShortenAutoLink checks that links whose commit or issue path
// follows a "#" are not shortened.
func TestMarkdown_ShortenAutoLink(t *testing.T) {
	conf.Server.ExternalURL = "http://localhost:3000/"
	NewSanitizer()

	for _, link := range []string{
		"http://localhost:3000/#issues/00",
		"http://localhost:3000/#commit/00",
	} {
		t.Run(link, func(t *testing.T) {
			want := "<p><a href=\"" + link + "\" rel=\"nofollow\">" + link + "</a></p>\n"
			assert.Equal(t, want, string(Markdown("<"+link+">", "/user/repo", nil)))
		})
	}
}

func TestMarkdown_Limits(t *testing.T) {
	conf.Server.ExternalURL = "http://localhost:3000/"
	NewSanitizer()

	t.Run("mentions render in linear output size", func(t *testing.T) {
		input := strings.Repeat("@a ", 10000)
		got := Markdown(input, "/user/repo", nil)
		assert.Less(t, len(got), 40*len(input))
		assert.Equal(t, `<p><a href="/a" rel="nofollow">@a</a> <a href="/ab" rel="nofollow">@ab</a></p>`+"\n", string(Markdown("@a @ab", "/user/repo", nil)))
	})

	t.Run("oversized document is shown as text", func(t *testing.T) {
		before := conf.UI.MaxDisplayFileSize
		conf.UI.MaxDisplayFileSize = 16
		defer func() { conf.UI.MaxDisplayFileSize = before }()

		assert.Equal(t, "<pre># Title &lt;b&gt; *emphasis*</pre>", string(Markdown("# Title <b> *emphasis*", "/user/repo", nil)))
		assert.Equal(t, "<h1 id=\"user-content-title\">Title</h1>\n", string(Markdown("# Title", "/user/repo", nil)))
		assert.Equal(t, "<pre># Title &lt;b&gt; *emphasis*</pre>", string(RawMarkdown([]byte("# Title <b> *emphasis*"), "")))
	})

	t.Run("byte order mark is ignored", func(t *testing.T) {
		assert.Equal(t, "<h1 id=\"user-content-title\">Title</h1>\n", string(Markdown("\ufeff# Title", "/user/repo", nil)))
		assert.Equal(t, "<h1 id=\"user-content-title\">Title</h1>\n", string(RawMarkdown([]byte("\ufeff# Title"), "")))
	})

	t.Run("fenced code keeps languages with symbols", func(t *testing.T) {
		assert.Equal(t, "<pre><code class=\"language-c++\">x\n</code></pre>\n", string(Markdown("```c++\nx\n```", "/user/repo", nil)))
	})

	t.Run("footnote ids differ between documents", func(t *testing.T) {
		a := string(Markdown("a[^1]\n\n[^1]: one", "/user/repo", nil))
		b := string(Markdown("b[^1]\n\n[^1]: two", "/user/repo", nil))
		assert.Contains(t, a, `id="user-content-`)
		assert.NotEqual(t, footnoteIDPrefix([]byte("a[^1]\n\n[^1]: one")), footnoteIDPrefix([]byte("b[^1]\n\n[^1]: two")))
		assert.Contains(t, b, footnoteIDPrefix([]byte("b[^1]\n\n[^1]: two"))+"fn:1")
	})
}

func TestMarkdown_Fractions(t *testing.T) {
	NewSanitizer()
	before := conf.Smartypants
	defer func() { conf.Smartypants = before }()
	conf.Smartypants.Enabled = true

	t.Run("only common fractions without FRACTIONS", func(t *testing.T) {
		conf.Smartypants.Fractions = false
		assert.Equal(t, "<p>½ cup, ¼th, ¾ths, 2/3, 1/2th, a1/2, <code>1/2</code></p>\n", string(Markdown("1/2 cup, 1/4th, 3/4ths, 2/3, 1/2th, a1/2, `1/2`", "/user/repo", nil)))
	})

	t.Run("any fraction with FRACTIONS", func(t *testing.T) {
		conf.Smartypants.Fractions = true
		assert.Equal(t, "<p><sup>2</sup>⁄<sub>3</sub>, <sup>12</sup>⁄<sub>345</sub>, 1/4th, 1/2/2020, 01/2</p>\n", string(Markdown("2/3, 12/345, 1/4th, 1/2/2020, 01/2", "/user/repo", nil)))
		// Documents with raw HTML are sanitized by SanitizeBytes instead.
		assert.Equal(t, "<p><b>x</b> <sup>2</sup>⁄<sub>3</sub></p>\n", string(Markdown("<b>x</b> 2/3", "/user/repo", nil)))
	})
}

func TestMarkdown_HeadingIDs(t *testing.T) {
	NewSanitizer()
	input := "# Hello, World!\n\n## Setup *fast*\n\n## Setup fast\n\n### setup-fast-1\n\n## 中文 标题\n\n## !!!\n"
	want := `<h1 id="user-content-hello-world">Hello, World!</h1>
<h2 id="user-content-setup-fast">Setup <em>fast</em></h2>
<h2 id="user-content-setup-fast-1">Setup fast</h2>
<h3 id="user-content-setup-fast-1-1">setup-fast-1</h3>
<h2 id="user-content-中文-标题">中文 标题</h2>
<h2>!!!</h2>
`
	assert.Equal(t, want, string(Markdown(input, "/user/repo", nil)))
	// Documents with raw HTML are sanitized by SanitizeBytes instead.
	assert.Equal(t, want+"<p><b>x</b></p>\n", string(Markdown(input+"\n<b>x</b>\n", "/user/repo", nil)))

	got := dariomark.Render([]byte(input), markdownOptions("", true)).Headings
	assert.Equal(t, []dariomark.Heading{
		{Level: 1, Text: "Hello, World!", ID: "user-content-hello-world"},
		{Level: 2, Text: "Setup fast", ID: "user-content-setup-fast"},
		{Level: 2, Text: "Setup fast", ID: "user-content-setup-fast-1"},
		{Level: 3, Text: "setup-fast-1", ID: "user-content-setup-fast-1-1"},
		{Level: 2, Text: "中文 标题", ID: "user-content-中文-标题"},
		{Level: 2, Text: "!!!", ID: ""},
	}, got)
}

func TestMarkdown_TaskLists(t *testing.T) {
	NewSanitizer()
	input := "- [ ] write\n- [x] review\n  details\n  - [ ] sub\n- plain\n\n3. [x] ship\n"
	want := `<ul class="contains-task-list">
<li class="task-list-item" data-source-lines="1-1"><input disabled="" type="checkbox"> write</li>
<li class="task-list-item" data-source-lines="2-4"><input checked="" disabled="" type="checkbox"> review<br>
details
<ul class="contains-task-list">
<li class="task-list-item" data-source-lines="4-4"><input disabled="" type="checkbox"> sub</li>
</ul>
</li>
<li>plain</li>
</ul>
<ol start="3" class="contains-task-list">
<li class="task-list-item" data-source-lines="7-7"><input checked="" disabled="" type="checkbox"> ship</li>
</ol>
`
	res := renderMarkdown([]byte(input), "/user/repo", nil, true, false)
	assert.True(t, res.Sanitized, "takes the fast path")
	assert.Equal(t, want, string(Markdown(input, "/user/repo", nil)))
	// Documents with raw HTML are sanitized by SanitizeBytes instead.
	assert.Equal(t, want+"<p><b>x</b></p>\n", string(Markdown(input+"\n<b>x</b>\n", "/user/repo", nil)))
}

// TestMarkdown_CodeInListItems checks code blocks inside list items. The
// expected block structure is what GitHub renders for the same input.
func TestMarkdown_CodeInListItems(t *testing.T) {
	NewSanitizer()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "fenced code without a blank line before it",
			input: "- item\n  ```go\n  func main() {}\n  ```\n- next\n",
			want:  "<ul>\n<li>item\n<pre><code class=\"language-go\">func main() {}\n</code></pre>\n</li>\n<li>next</li>\n</ul>\n",
		},
		{
			name:  "fenced code in a nested item",
			input: "- a\n  - b\n\n    ```go\n    nested()\n    ```\n\n  - c\n",
			want:  "<ul>\n<li>a\n<ul>\n<li>\n<p>b</p>\n<pre><code class=\"language-go\">nested()\n</code></pre>\n</li>\n<li>\n<p>c</p>\n</li>\n</ul>\n</li>\n</ul>\n",
		},
		{
			name:  "fenced code in ordered steps",
			input: "1. Clone:\n\n   ```sh\n   git clone https://example.com/r.git\n   ```\n\n2. Build:\n\n   ```sh\n   go build ./...\n   ```\n\n   Then run it.\n\n3. Done.\n",
			want:  "<ol>\n<li>\n<p>Clone:</p>\n<pre><code class=\"language-sh\">git clone https://example.com/r.git\n</code></pre>\n</li>\n<li>\n<p>Build:</p>\n<pre><code class=\"language-sh\">go build ./...\n</code></pre>\n<p>Then run it.</p>\n</li>\n<li>\n<p>Done.</p>\n</li>\n</ol>\n",
		},
		{
			name:  "item that starts with a fence",
			input: "- ```go\n  first()\n  ```\n- b\n",
			want:  "<ul>\n<li>\n<pre><code class=\"language-go\">first()\n</code></pre>\n</li>\n<li>b</li>\n</ul>\n",
		},
		{
			name:  "indented code in a nested item",
			input: "- a\n  - b\n\n        nested indented\n",
			want:  "<ul>\n<li>a\n<ul>\n<li>\n<p>b</p>\n<pre><code>nested indented\n</code></pre>\n</li>\n</ul>\n</li>\n</ul>\n",
		},
		{
			name:  "fence that is not indented ends the list",
			input: "- item\n\n```go\noutside()\n```\n",
			want:  "<ul>\n<li>item</li>\n</ul>\n<pre><code class=\"language-go\">outside()\n</code></pre>\n",
		},
		{
			name:  "less indented line ends the item and its code",
			input: "- item\n\n  ```\n  a\n b\nc\n  ```\n",
			want:  "<ul>\n<li>\n<p>item</p>\n<pre><code>a\n</code></pre>\n</li>\n</ul>\n<p>b\nc</p>\n<pre><code></code></pre>\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, string(MarkdownFile(test.input, "/user/repo", nil)))
		})
	}
}

// FuzzMarkdown checks that documents sanitized while rendering come out as
// rendering them without sanitizing and then sanitizing them with
// SanitizeBytes would, and that SanitizeBytes leaves them unchanged.
func FuzzMarkdown(f *testing.F) {
	conf.Server.ExternalURL = "http://localhost:3000/"
	NewSanitizer()
	for _, s := range []string{
		"# Title\n\n*a* **b** `c` [d](e \"f\") ![g](h.png)\n",
		"> [!NOTE]\n> x\n\n- [ ] a\n- [x] b\n\n| a | b |\n|:-|-:|\n| c | d |\n",
		"x[^1] ~~y~~ www.example.com a@b.co http://x.y/z\n\n[^1]: note\n",
		"@user #12 user/repo#7 d8a994ef24 ABC-12 http://localhost:3000/user/repo/issues/1\n",
		"[x](javascript:alert(1)) ![y](data:image/png;base64,AAA) <a@b.c> [z](//evil.example.com \"t\\\" onclick=\\\"x\")\n",
		"```c++\nx\n```\n\n    @user\n\n<b>raw</b> <textarea>@user</textarea>\n",
		"a\nb :tada:\nc  \nd\\\ne\n",
		"---\nt: <b>@user</b> #1 :tada: 'q\"\nl: [a, {b: ~}]\n---\n- [ ] x\n",
		"---\nt: [x\n---\n<b>y</b>\n",
	} {
		f.Add(s)
	}
	const urlPrefix = "/user/repo"
	f.Fuzz(func(t *testing.T, s string) {
		body := []byte(s)
		// Both line break modes, comments and files, chosen by the input.
		hardWraps := len(s)%2 == 0
		file := !hardWraps
		res := renderMarkdown(body, urlPrefix, trackerMetas(), hardWraps, file)
		got := res.HTML
		if !res.Sanitized {
			SanitizeBytes(got)
			return
		}

		opts := markdownOptions(urlPrefix, hardWraps)
		opts.FootnoteIDPrefix = footnoteIDPrefix(body)
		opts.PostProcess = true
		opts.SpecialLinks = func(text []byte) []dariomark.SpecialLink {
			return findSpecialLinks(text, urlPrefix, trackerMetas(), nil)
		}
		opts.Emoji = emoji
		if file {
			opts.FrontMatter = true
		}
		want := SanitizeBytes(dariomark.Render(body, opts).HTML)
		if string(got) != string(want) {
			t.Fatalf("sanitizing while rendering differs from SanitizeBytes for %q\ngot  %q\nwant %q", s, got, want)
		}
		if again := SanitizeBytes(got); string(again) != string(got) {
			t.Fatalf("SanitizeBytes changes the output for %q\ngot   %q\nagain %q", s, got, again)
		}
	})
}

func TestMarkdownFile_FrontMatter(t *testing.T) {
	conf.Server.ExternalURL = "http://localhost:3000/"
	NewSanitizer()

	const table = "<table>\n<tbody>\n" +
		"<tr>\n<th>title</th>\n<td>Hello &lt;script&gt;</td>\n</tr>\n" +
		"<tr>\n<th>tags</th>\n<td><table><tbody><tr><td><div>a</div></td><td><div>b</div></td></tr></tbody></table></td>\n</tr>\n" +
		"<tr>\n<th>authors</th>\n<td><table><tbody><tr><td><div><table><thead><tr><th>name</th><th>url</th></tr></thead>" +
		"<tbody><tr><td><div>Unknwon</div></td><td><div></div></td></tr></tbody></table></div></td></tr></tbody></table></td>\n</tr>\n" +
		"<tr>\n<th>draft</th>\n<td></td>\n</tr>\n" +
		"</tbody>\n</table>\n"
	const frontMatter = "---\ntitle: Hello <script>\ntags: [a, b]\nauthors:\n  - name: Unknwon\n    url:\ndraft: ~\n---\n"
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "table",
			input: frontMatter + "# Hello @unknwon :tada:\n",
			want:  table + "<h1 id=\"user-content-hello-unknwon-tada\">Hello <a href=\"/unknwon\" rel=\"nofollow\">@unknwon</a> 🎉</h1>\n",
		},
		{
			name:  "raw HTML",
			input: frontMatter + "<b>x</b>\n",
			want:  table + "<p><b>x</b></p>\n",
		},
		{
			name:  "CRLF, trailing spaces and no final line ending",
			input: "--- \t\r\na: 1\r\n---\t",
			want:  "<table>\n<tbody>\n<tr>\n<th>a</th>\n<td>1</td>\n</tr>\n</tbody>\n</table>\n",
		},
		{
			name:  "special links stay text",
			input: "---\na: \"@unknwon #1 :tada:\"\n---\n",
			want:  "<table>\n<tbody>\n<tr>\n<th>a</th>\n<td>@unknwon #1 :tada:</td>\n</tr>\n</tbody>\n</table>\n",
		},
		{name: "unclosed", input: "---\na: 1\n\nb\n", want: "<hr>\n<p>a: 1</p>\n<p>b</p>\n"},
		{name: "closed by dots", input: "---\na: 1\n...\n", want: "<hr>\n<p>a: 1\n...</p>\n"},
		{name: "empty", input: "---\n---\n", want: "<hr>\n<hr>\n"},
		{name: "not a mapping", input: "---\n- a\n---\n", want: "<hr>\n<ul>\n<li>a</li>\n</ul>\n<hr>\n"},
		{name: "not at the start", input: "\n---\na: 1\n---\n", want: "<hr>\n<h2 id=\"user-content-a-1\">a: 1</h2>\n"},
		{
			name:  "invalid YAML",
			input: "---\na: [<b>\n---\nb\n",
			want:  "<pre><code class=\"language-yaml\">---\na: [&lt;b&gt;\n---\n</code></pre>\n<p>b</p>\n",
		},
		{
			// Aliases are not allowed, since they could multiply nodes, e.g.,
			// "billion laughs", or contain themselves.
			name:  "aliases",
			input: "---\na: &a [x, x]\nb: [*a, *a]\n---\n",
			want:  "<pre><code class=\"language-yaml\">---\na: &amp;a [x, x]\nb: [*a, *a]\n---\n</code></pre>\n",
		},
		{
			name:  "too deep",
			input: "---\na: " + strings.Repeat("[", 17) + strings.Repeat("]", 17) + "\n---\n",
			want:  "<pre><code class=\"language-yaml\">---\na: " + strings.Repeat("[", 17) + strings.Repeat("]", 17) + "\n---\n</code></pre>\n",
		},
		{
			name:  "too large",
			input: "---\na: " + strings.Repeat("x", 64<<10) + "\n---\n",
			want:  "<hr>\n<h2 id=\"user-content-a-" + strings.Repeat("x", 64<<10) + "\">a: " + strings.Repeat("x", 64<<10) + "</h2>\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := MarkdownFile(test.input, "/user/repo", nil)
			assert.Equal(t, test.want, string(got))
			assert.Equal(t, string(got), string(SanitizeBytes(got)), "survives sanitizing")
		})
	}

	t.Run("task lists keep their source lines", func(t *testing.T) {
		got := string(MarkdownFile("---\r\na: 1\r\n---\r\n- [ ] x\r\n- [x] y\r\n", "/user/repo", nil))
		assert.Contains(t, got, `<li class="task-list-item" data-source-lines="4-4">`)
		assert.Contains(t, got, `<li class="task-list-item" data-source-lines="5-5">`)
	})
	t.Run("comments", func(t *testing.T) {
		assert.Equal(t, "<hr>\n<h2 id=\"user-content-a-1\">a: 1</h2>\n", string(Markdown("---\na: 1\n---\n", "/user/repo", nil)))
	})
}
