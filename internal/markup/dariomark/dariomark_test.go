package dariomark

import (
	"bytes"
	"encoding/json"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSpec checks the examples of the CommonMark 0.31.2 specification, from
// https://spec.commonmark.org/0.31.2/spec.json.
func TestSpec(t *testing.T) {
	data, err := os.ReadFile("testdata/spec.json")
	require.NoError(t, err)
	var examples []struct {
		Markdown string `json:"markdown"`
		HTML     string `json:"html"`
		Example  int    `json:"example"`
		Section  string `json:"section"`
	}
	require.NoError(t, json.Unmarshal(data, &examples))
	require.Len(t, examples, 652)

	opts := &Options{XHTML: true}
	for _, ex := range examples {
		t.Run(strconv.Itoa(ex.Example), func(t *testing.T) {
			assert.Equal(t, ex.HTML, string(HTML([]byte(ex.Markdown), opts)), "%s\n%q", ex.Section, ex.Markdown)
		})
	}
}

func TestAlerts(t *testing.T) {
	note := func(body string) string {
		return "<div class=\"markdown-alert markdown-alert-note\">\n<p class=\"markdown-alert-title\">Note</p>\n" + body + "</div>\n"
	}
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "note", input: "> [!NOTE]\n> Hello", want: note("<p>Hello</p>\n")},
		{name: "tip in lowercase", input: "> [!tip]\n> Hello", want: "<div class=\"markdown-alert markdown-alert-tip\">\n<p class=\"markdown-alert-title\">Tip</p>\n<p>Hello</p>\n</div>\n"},
		{name: "important with trailing whitespace", input: "> [!Important] \t\n> Hello *x*", want: "<div class=\"markdown-alert markdown-alert-important\">\n<p class=\"markdown-alert-title\">Important</p>\n<p>Hello <em>x</em></p>\n</div>\n"},
		{name: "warning with a list", input: "> [!WARNING]\n>\n> - a\n> - b", want: "<div class=\"markdown-alert markdown-alert-warning\">\n<p class=\"markdown-alert-title\">Warning</p>\n<ul>\n<li>a</li>\n<li>b</li>\n</ul>\n</div>\n"},
		{name: "caution with paragraphs", input: "> [!CAUTION]\n> a\n>\n> b", want: "<div class=\"markdown-alert markdown-alert-caution\">\n<p class=\"markdown-alert-title\">Caution</p>\n<p>a</p>\n<p>b</p>\n</div>\n"},
		{name: "no space after marker", input: ">[!NOTE]\n>Hello", want: note("<p>Hello</p>\n")},
		{name: "heading", input: "> [!NOTE]\n> # Title", want: note("<h1>Title</h1>\n")},
		{name: "lazy continuation", input: "> [!NOTE]\nlazy", want: note("<p>lazy</p>\n")},
		{name: "after a paragraph", input: "x\n> [!NOTE]\n> Hello", want: "<p>x</p>\n" + note("<p>Hello</p>\n")},

		{name: "text on marker line", input: "> [!NOTE] text\n> Hello", want: "<blockquote>\n<p>[!NOTE] text\nHello</p>\n</blockquote>\n"},
		{name: "no content", input: "> [!NOTE]", want: "<blockquote>\n<p>[!NOTE]</p>\n</blockquote>\n"},
		{name: "only blank lines after marker", input: "> [!NOTE]\n>", want: "<blockquote>\n<p>[!NOTE]</p>\n</blockquote>\n"},
		{name: "unknown type", input: "> [!FOO]\n> Hello", want: "<blockquote>\n<p>[!FOO]\nHello</p>\n</blockquote>\n"},
		{name: "space inside marker", input: "> [! NOTE]\n> Hello", want: "<blockquote>\n<p>[! NOTE]\nHello</p>\n</blockquote>\n"},
		{name: "escaped marker", input: "> \\[!NOTE]\n> Hello", want: "<blockquote>\n<p>[!NOTE]\nHello</p>\n</blockquote>\n"},
		{name: "in a list item", input: "- > [!NOTE]\n  > Hello", want: "<ul>\n<li>\n<blockquote>\n<p>[!NOTE]\nHello</p>\n</blockquote>\n</li>\n</ul>\n"},
		{name: "nested blockquote", input: "> > [!NOTE]\n> > Hello", want: "<blockquote>\n<blockquote>\n<p>[!NOTE]\nHello</p>\n</blockquote>\n</blockquote>\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, string(HTML([]byte(test.input), &Options{Alerts: true})))
		})
	}
}

func FuzzRender(f *testing.F) {
	for _, s := range []string{
		"# Title\n\n*a* **b** `c` [d](e \"f\") ![g](h)\n",
		"> [!NOTE]\n> x\n\n- [ ] a\n- [x] b\n\n| a | b |\n|:-|-:|\n| c | d |\n",
		"x[^1] ~~y~~ www.example.com a@b.co http://x.y/z\n\n[^1]: note\n",
		"<div>\n<a href=\"x\">http://x.y</a>\n</div>\n<!-- c --> <b>x</b>\n",
		"\"quotes\" -- dashes --- 'single' ...\n",
		"[x](<a>\"t\") [y](",
	} {
		f.Add(s)
	}
	opts := gogsTestOptions()
	f.Fuzz(func(t *testing.T, s string) {
		first := Render([]byte(s), opts)
		second := Render([]byte(s), opts)
		if !bytes.Equal(first.HTML, second.HTML) || first.Sanitized != second.Sanitized {
			t.Fatalf("output is not deterministic for %q", s)
		}
	})
}

// gogsTestOptions returns options like those of Gogs, with the sanitizing of
// documents without raw HTML.
func gogsTestOptions() *Options {
	return &Options{
		Tables:        true,
		Strikethrough: true,
		TaskLists:     true,
		Linkify:       true,
		LinkifyURL: func(b []byte) int {
			return len(regexp.MustCompile(`^https?://[a-z.]+`).Find(b))
		},
		LinkifyAfterPunct: true,
		Footnotes:         true,
		FootnoteIDPrefix:  "user-content-",
		Alerts:            true,
		Typographer:       Typographer{Enabled: true, Dashes: true},
		RelativeLinks:     true,
		LinkBase:          "/user/repo",
		IsLink:            func(dest []byte) bool { return bytes.Contains(dest, []byte("://")) },
		Gogs: &GogsOptions{
			URLPrefix:    "/user/repo",
			SpecialLinks: func(text []byte) []byte { return text },
			Sanitizer: &Sanitizer{
				AllowURLScheme: func(u *url.URL) bool { return u.Scheme == "http" || u.Scheme == "https" },
				Text:           regexp.MustCompile(`^[\p{L}\p{N}\s\-_',\[\]!\./\\\(\)]*$`),
				CodeClass:      regexp.MustCompile(`^language-\w+$`),
			},
		},
	}
}
