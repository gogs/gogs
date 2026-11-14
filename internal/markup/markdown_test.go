package markup

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

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
			name:  "commit",
			input: "Fixed in d8a994ef243349f321568f9e36d5c3f444b99cae.",
			want:  "<p>Fixed in <a href=\"/user/repo/commit/d8a994ef243349f321568f9e36d5c3f444b99cae\" rel=\"nofollow\"><code>d8a994ef24</code></a>.</p>\n",
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
			want:  "<p><a href=\"https://example.com\" rel=\"nofollow\"><img src=\"badge.svg\" alt=\"badge\"></a></p>\n",
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
			_, sanitized := renderMarkdown([]byte(test.input), "/user/repo", trackerMetas())
			assert.True(t, sanitized, "takes the fast path")
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
		// Documents with raw HTML are sanitized by bluemonday instead.
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
	// Documents with raw HTML are sanitized by bluemonday instead.
	assert.Equal(t, want+"<p><b>x</b></p>\n", string(Markdown(input+"\n<b>x</b>\n", "/user/repo", nil)))

	got := dariomark.Render([]byte(input), markdownOptions("")).Headings
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
<li class="task-list-item" data-source-lines="2-4"><input checked="" disabled="" type="checkbox"> review
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
	_, sanitized := renderMarkdown([]byte(input), "/user/repo", nil)
	assert.True(t, sanitized, "takes the fast path")
	assert.Equal(t, want, string(Markdown(input, "/user/repo", nil)))
	// Documents with raw HTML are sanitized by bluemonday instead.
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
			assert.Equal(t, test.want, string(Markdown(test.input, "/user/repo", nil)))
		})
	}
}

// FuzzMarkdown checks that documents sanitized while rendering come out as
// rendering them without sanitizing and then sanitizing them with bluemonday
// would, and that bluemonday leaves them unchanged.
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
	} {
		f.Add(s)
	}
	const urlPrefix = "/user/repo"
	f.Fuzz(func(t *testing.T, s string) {
		body := []byte(s)
		got, sanitized := renderMarkdown(body, urlPrefix, trackerMetas())
		if !sanitized {
			SanitizeBytes(got)
			return
		}

		opts := markdownOptions(urlPrefix)
		opts.FootnoteIDPrefix = footnoteIDPrefix(body)
		opts.PostProcess = true
		opts.SpecialLinks = func(text []byte) []dariomark.SpecialLink {
			return findSpecialLinks(text, urlPrefix, trackerMetas())
		}
		want := SanitizeBytes(dariomark.Render(body, opts).HTML)
		if string(got) != string(want) {
			t.Fatalf("sanitizing while rendering differs from bluemonday for %q\ngot  %q\nwant %q", s, got, want)
		}
		if again := SanitizeBytes(got); string(again) != string(got) {
			t.Fatalf("bluemonday changes the output for %q\ngot   %q\nagain %q", s, got, again)
		}
	})
}
