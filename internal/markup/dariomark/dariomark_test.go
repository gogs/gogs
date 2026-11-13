package dariomark

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSpec checks the examples of the CommonMark 0.31.2 specification, from
// https://spec.commonmark.org/0.31.2/spec.json.
func TestSpec(t *testing.T) {
	// Dariomark renders these examples the way GitHub does instead:
	// numeric character references of 8 digits, currency symbols that are
	// not punctuation, and strong emphasis directly inside strong emphasis.
	github := map[int]string{
		28:  "<p>&amp;nbsp &amp;x; &amp;#; &amp;#x;\n\ufffd\n&amp;#abcdef0;\n&amp;ThisIsNotDefined; &amp;hi?;</p>\n",
		354: "<p>*$*alpha.</p>\n<p><em>\u00a3</em>bravo.</p>\n<p><em>\u20ac</em>charlie.</p>\n",
		389: "<p><strong>foo, bar, baz</strong></p>\n",
		417: "<p>foo<strong>bar</strong>***baz</p>\n",
		425: "<p><strong>foo bar baz</strong></p>\n",
		426: "<p><strong>foo bar</strong></p>\n",
		427: "<p><strong>foo bar</strong></p>\n",
		464: "<p><strong>foo</strong></p>\n",
		465: "<p><strong>foo</strong></p>\n",
		466: "<p><strong>foo</strong></p>\n",
		468: "<p><em><strong>foo</strong></em></p>\n",
	}

	data, err := os.ReadFile("testdata/commonmark_spec.json")
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
			want, ok := github[ex.Example]
			if !ok {
				want = ex.HTML
			}
			assert.Equal(t, want, string(HTML([]byte(ex.Markdown), opts)), "%s\n%q", ex.Section, ex.Markdown)
		})
	}
}

// TestGFMSpec checks the extension examples of the GitHub Flavored Markdown
// 0.29 specification, from
// https://github.com/github/cmark-gfm/blob/0.29.0.gfm.13/test/spec.txt.
func TestGFMSpec(t *testing.T) {
	examples := readGFMExtensionExamples(t, "testdata/gfm_spec.txt")
	require.Len(t, examples, 24)

	// Task lists render with classes and source lines, see
	// TestTaskListAttributes.
	taskAttrs := regexp.MustCompile(` class="contains-task-list"| class="task-list-item" data-source-lines="\d+-\d+"`)

	opts := &Options{Tables: true, Strikethrough: true, TaskLists: true, Linkify: true}
	for _, ex := range examples {
		t.Run(strconv.Itoa(ex.Example), func(t *testing.T) {
			if ex.Extension == "tagfilter" {
				t.Skip("HTML is sanitized with bluemonday instead of filtering tags")
			}
			got := string(HTML([]byte(ex.Markdown), opts))
			got = taskAttrs.ReplaceAllString(got, "")
			assert.Equal(t, ex.HTML, got, "%s\n%q", ex.Section, ex.Markdown)
		})
	}
}

type gfmExample struct {
	Markdown, HTML string
	// Example is the number of the example in the specification.
	Example int
	Section string
	// Extension is the extension that the specification names for the
	// example, or "disabled" for the task list examples, which cmark-gfm does
	// not test.
	Extension string
}

// readGFMExtensionExamples reads the examples in the extension sections of the
// specification in name, in the format of spec.txt of cmark-gfm: an example
// is the Markdown and the HTML, separated by a line ".", between fences of 32
// backticks, with "→" in place of tabs.
func readGFMExtensionExamples(t *testing.T, name string) []gfmExample {
	data, err := os.ReadFile(name)
	require.NoError(t, err)

	const fence = "````````````````````````````````"
	var (
		examples []gfmExample
		section  string
		number   int
		ex       *gfmExample
		part     *strings.Builder
		markdown strings.Builder
		html     strings.Builder
	)
	for _, line := range strings.SplitAfter(string(data), "\n") {
		switch {
		case ex == nil && strings.HasPrefix(line, fence+" example"):
			number++
			ex = &gfmExample{Example: number, Section: section, Extension: strings.TrimSpace(strings.TrimPrefix(line, fence+" example"))}
			markdown.Reset()
			html.Reset()
			part = &markdown
		case ex == nil:
			if strings.HasPrefix(line, "#") {
				section = strings.TrimSpace(strings.TrimLeft(line, "#"))
			}
		case line == fence+"\n" || line == fence:
			if strings.HasSuffix(ex.Section, "(extension)") {
				ex.Markdown = strings.ReplaceAll(markdown.String(), "→", "\t")
				ex.HTML = strings.ReplaceAll(html.String(), "→", "\t")
				examples = append(examples, *ex)
			}
			ex = nil
		case line == ".\n" && part == &markdown:
			part = &html
		default:
			part.WriteString(line)
		}
	}
	return examples
}

// TestGitHubRendering checks cases where GitHub's rendering departs from the
// specifications, with the output of github.com.
func TestGitHubRendering(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "www link before \";\"", input: "see www.example.com; then", want: "<p>see <a href=\"http://www.example.com\">www.example.com</a>; then</p>\n"},
		{name: "www link before a quote", input: "see www.example.com\". x", want: "<p>see <a href=\"http://www.example.com\">www.example.com</a>&quot;. x</p>\n"},
		{name: "emphasized www link", input: "*www.x.m*'", want: "<p><em><a href=\"http://www.x.m\">www.x.m</a></em>'</p>\n"},
		{name: "entity after URL", input: "see http://a.b/c&x; y", want: "<p>see <a href=\"http://a.b/c\">http://a.b/c</a>&amp;x; y</p>\n"},
		{name: "underscore ending the text", input: "www.a.b_", want: "<p><a href=\"http://www.a.b\">www.a.b</a>_</p>\n"},
		{name: "underscore before more text", input: "www.a.b_ x", want: "<p>www.a.b_ x</p>\n"},
		{name: "short www domain", input: "www.b", want: "<p><a href=\"http://www.b\">www.b</a></p>\n"},
		{name: "package version", input: "new in i18next@15.1.0", want: "<p>new in i18next@15.1.0</p>\n"},
		{name: "task marker alone", input: "- [ ] ", want: "<ul>\n<li>[ ]</li>\n</ul>\n"},
		{name: "task marker before a nested list", input: "- [ ]\n  - b", want: "<ul class=\"contains-task-list\">\n<li class=\"task-list-item\" data-source-lines=\"1-2\"><input disabled=\"\" type=\"checkbox\" /> \n<ul>\n<li>b</li>\n</ul>\n</li>\n</ul>\n"},
		{name: "tildes of another length", input: "~~approx ~5 min~~", want: "<p>~~approx ~5 min~~</p>\n"},
		{name: "line break in alt text", input: "![a\nb](u)", want: "<p><img src=\"u\" alt=\"a b\" /></p>\n"},
		{name: "closing tag after a list item", input: "- a\n</Note>\n", want: "<ul>\n<li>a</li>\n</ul>\n</Note>\n"},
		{name: "tab before a line ending", input: "a\t\nb", want: "<p>a\nb</p>\n"},
		{name: "lowercase declaration", input: "<!doctype html>", want: "<p>&lt;!doctype html&gt;</p>\n"},
		{name: "tilde next to emphasis", input: "*<*~x", want: "<p>*&lt;*~x</p>\n"},
	}
	opts := &Options{XHTML: true, Tables: true, Strikethrough: true, TaskLists: true, Linkify: true}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, string(HTML([]byte(test.input), opts)))
		})
	}
}

func TestTaskListAttributes(t *testing.T) {
	got := string(HTML([]byte("- [ ] a\n- [x] b\n  c\n- d\n"), &Options{TaskLists: true}))
	assert.Equal(t, "<ul class=\"contains-task-list\">\n"+
		"<li class=\"task-list-item\" data-source-lines=\"1-1\"><input disabled=\"\" type=\"checkbox\"> a</li>\n"+
		"<li class=\"task-list-item\" data-source-lines=\"2-3\"><input checked=\"\" disabled=\"\" type=\"checkbox\"> b\nc</li>\n"+
		"<li>d</li>\n</ul>\n", got)
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
	opts := postProcessTestOptions()
	f.Fuzz(func(t *testing.T, s string) {
		first := Render([]byte(s), opts)
		second := Render([]byte(s), opts)
		if !bytes.Equal(first.HTML, second.HTML) || first.Sanitized != second.Sanitized {
			t.Fatalf("output is not deterministic for %q", s)
		}
	})
}

// postProcessTestOptions returns options like those of markup.Markdown, with
// post-processing and the sanitizing of documents without raw HTML.
func postProcessTestOptions() *Options {
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
		PostProcess:       true,
		SpecialLinks:      func([]byte) []SpecialLink { return nil },
		Sanitizer: &Sanitizer{
			AllowURLScheme: func(u *url.URL) bool { return u.Scheme == "http" || u.Scheme == "https" },
			Text:           regexp.MustCompile(`^[\p{L}\p{N}\s\-_',\[\]!\./\\\(\)]*$`),
			CodeClass:      regexp.MustCompile(`^language-\w+$`),
		},
	}
}

func TestFootnoteLabels(t *testing.T) {
	opts := &Options{Footnotes: true}
	tests := []struct {
		name  string
		input string
		refs  int
	}{
		{name: "case", input: "A[^note-1] B[^Note-1]\n\n[^note-1]: n", refs: 2},
		{name: "whitespace", input: "A[^a  b] B[^a b]\n\n[^a b]: n", refs: 2},
		{name: "Unicode case folding", input: "A[^ss] B[^ẞ]\n\n[^ß]: n", refs: 2},
		{name: "different labels", input: "A[^a] B[^b]\n\n[^a]: n", refs: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out := string(HTML([]byte(test.input), opts))
			assert.Equal(t, test.refs, strings.Count(out, `class="footnote-ref"`), out)
			assert.Equal(t, 1, strings.Count(out, `<li id="fn:1">`), out)
		})
	}
}

func TestTablePadding(t *testing.T) {
	opts := &Options{Tables: true}
	t.Run("short rows are filled", func(t *testing.T) {
		got := string(HTML([]byte("| a | b |\n|:-|-:|\n| c |\n"), opts))
		assert.Equal(t, "<table>\n<thead>\n<tr>\n<th align=\"left\">a</th>\n<th align=\"right\">b</th>\n</tr>\n</thead>\n<tbody>\n<tr>\n<td align=\"left\">c</td>\n<td></td>\n</tr>\n</tbody>\n</table>\n", got)
	})

	t.Run("filler cells are bounded by the input size", func(t *testing.T) {
		const n = 3000
		in := strings.Repeat("|a", n) + "|\n" + strings.Repeat("|-", n) + "|\n" + strings.Repeat("|b\n", n) + "\nafter"
		start := time.Now()
		got := string(HTML([]byte(in), opts))
		assert.Less(t, time.Since(start), 5*time.Second)
		assert.Less(t, len(got), 40*len(in))
		// Every written cell still shows, and the rest of the document is
		// rendered normally.
		assert.Equal(t, n, strings.Count(got, "<td>b</td>"))
		assert.True(t, strings.HasSuffix(got, "</table>\n<p>after</p>\n"))
		// The first rows are filled until the budget runs out.
		assert.Contains(t, got, "<tr>\n<td>b</td>\n<td></td>\n")
		assert.Contains(t, got, "<tr>\n<td>b</td>\n</tr>\n")
	})
}

func TestReferenceBudget(t *testing.T) {
	opts := &Options{}
	t.Run("short references are unaffected", func(t *testing.T) {
		in := "[a]: /u \"t\"\n\n" + strings.Repeat("[a] ", 100000)
		got := string(HTML([]byte(in), opts))
		assert.Equal(t, 100000, strings.Count(got, `<a href="/u" title="t">a</a>`))
	})

	for _, use := range []string{"[a]", "![a]"} {
		t.Run("long destination used as "+use, func(t *testing.T) {
			const n = 1 << 16
			in := "[a]: /" + strings.Repeat("x", n/2) + "\n\n" + strings.Repeat(use, n/len(use)) + "\n\n*after*"
			start := time.Now()
			got := string(HTML([]byte(in), opts))
			assert.Less(t, time.Since(start), 5*time.Second)
			assert.Less(t, len(got), 40*len(in))
			// The references within the budget are links, the rest stay text.
			assert.True(t, strings.HasPrefix(got, "<p>"+map[string]string{"[a]": "<a href", "![a]": "<img src"}[use]))
			assert.True(t, strings.HasSuffix(got, use+"</p>\n<p><em>after</em></p>\n"))
		})
	}
}

func TestAngleDestination(t *testing.T) {
	opts := &Options{}
	// "<c>" is raw HTML.
	assert.Equal(t, "<p>[a](&lt;b<c>)</p>\n", string(HTML([]byte("[a](<b<c>)"), opts)))
	assert.Equal(t, "<p>[a]</p>\n<p>[a]: &lt;b<c></p>\n", string(HTML([]byte("[a]\n\n[a]: <b<c>"), opts)))
	assert.Equal(t, "<p><a href=\"b%3Cc\">a</a></p>\n", string(HTML([]byte("[a](<b\\<c>)"), opts)))

	in := []byte(strings.Repeat("[a](<", 1<<16))
	start := time.Now()
	HTML(in, opts)
	assert.Less(t, time.Since(start), 2*time.Second)
}

func TestEmphasisDelimiterSearch(t *testing.T) {
	// Every second tilde closer searches for an opener down to the first
	// tilde, past the "*" runs that never close. Without the first tilde,
	// the search stops at the previous closer, so the time of that document
	// is the bound, whatever the speed of the machine.
	const n = 1 << 15
	body := strings.Repeat("*x ", n) + strings.Repeat("x~~x ", n)
	render := func(in string) (string, time.Duration) {
		start := time.Now()
		out := HTML([]byte(in), &Options{Strikethrough: true})
		return string(out), time.Since(start)
	}
	_, bound := render("x " + body)
	got, elapsed := render("~x " + body)
	assert.Less(t, elapsed, 10*bound+50*time.Millisecond)
	assert.Equal(t, n/2, strings.Count(got, "<del>"))
}

func TestImageLinkEscaping(t *testing.T) {
	opts := postProcessTestOptions()
	opts.Sanitizer = nil
	got := string(Render([]byte("![a&quot; id=&quot;x](x.png?a=1&b=2)"), opts).HTML)
	assert.Equal(t, "<p><a href=\"/user/repo/x.png?a=1&amp;b=2\"><img src=\"/user/repo/x.png?a=1&amp;b=2\" alt=\"a&#34; id=&#34;x\"></a></p>\n", got)
}

// TestStreaming checks that large documents, which are rendered while they
// are parsed, render as they would in one piece.
func TestStreaming(t *testing.T) {
	parts := []string{
		"# h\n", "a\n=\n", "para *em\n", "more_ text**\n", "\n", "> q\n", "> [!NOTE]\n> x\n", "- a\n", "  - b\n",
		"    code\n", "```go\nf\n```\n", "| a | b |\n|---|---|\n| c |\n", "- [ ] t\n", "lazy\n", "***\n",
		"www.x.com a@b.co\n", "[l](u) ![i](j)\n", "`co\nde`\n", "\"q\" 'q' --\n", "<b>raw</b>\n", "<div>\n\n",
	}
	defer func(size int) { streamMinSize = size }(streamMinSize)
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 40; i++ {
		var b strings.Builder
		for b.Len() < 64<<10 {
			// Long runs of one block make the document drop the lines of
			// rendered blocks.
			u := parts[rng.Intn(len(parts))]
			b.WriteString(strings.Repeat(u, 1+rng.Intn(4)*rng.Intn(1500)))
		}
		in := []byte(b.String())
		for _, opts := range []*Options{postProcessTestOptions(), {Tables: true, Alerts: true, HeadingIDs: true}} {
			streamMinSize = len(in) + 1
			want := Render(in, opts)
			wantHTML := string(want.HTML)
			streamMinSize = 0
			got := Render(in, opts)
			require.Equal(t, wantHTML, string(got.HTML))
			require.Equal(t, want.Sanitized, got.Sanitized)
			require.Equal(t, want.Headings, got.Headings)
		}
	}
}

func TestNestingLimit(t *testing.T) {
	tests := []struct {
		name  string
		input string
		tag   string
		inner string
	}{
		{name: "block quotes", input: strings.Repeat("> ", maxNesting+1) + "a", tag: "<blockquote>", inner: "<p>&gt; a</p>"},
		{name: "list items", input: strings.Repeat("- ", maxNesting+1) + "a", tag: "<li>", inner: "<li>- a</li>"},
		{name: "list item in block quotes", input: strings.Repeat("> ", maxNesting) + "- a", tag: "<blockquote>", inner: "<p>- a</p>"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := string(HTML([]byte(test.input), &Options{}))
			// The markers past the limit are text of the innermost block.
			assert.Equal(t, maxNesting, strings.Count(got, test.tag))
			assert.Contains(t, got, test.inner)
		})
	}
}
