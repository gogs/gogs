package markup_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	. "gogs.io/gogs/internal/markup"
)

func TestSanitizer(t *testing.T) {
	NewSanitizer()
	tests := []struct {
		input  string
		expVal string
	}{
		// Regular
		{input: `<a onblur="alert(secret)" href="http://www.google.com">Google</a>`, expVal: `<a href="http://www.google.com" rel="nofollow">Google</a>`},

		// Code highlighting class
		{input: `<code class="random string"></code>`, expVal: `<code></code>`},
		{input: `<code class="language-random ui tab active menu attached animating sidebar following bar center"></code>`, expVal: `<code></code>`},
		{input: `<code class="language-go"></code>`, expVal: `<code class="language-go"></code>`},
		{input: `<code class="language-c++"></code>`, expVal: `<code class="language-c++"></code>`},
		{input: `<code class="language-c#"></code>`, expVal: `<code class="language-c#"></code>`},

		// Keyboard keys
		{input: `<kbd>Ctrl</kbd>+<kbd class="ui button" style="color:red" onclick="alert(1)">C</kbd>`, expVal: `<kbd>Ctrl</kbd>+<kbd>C</kbd>`},

		// Block alignment
		{input: `<div align="center"><p align="Right">x</p><h1 align="left">t</h1></div>`, expVal: `<div align="center"><p align="Right">x</p><h1 align="left">t</h1></div>`},
		{input: `<div align="center;color:red"></div><span align="center"></span>`, expVal: `<div></div><span></span>`},

		// Image sizes
		{input: `<img src="a.png" width="201px" height="50%">`, expVal: `<img src="a.png" width="201px" height="50%">`},
		{input: `<img src="a.png" width="20em" height="1px;color:red">`, expVal: `<img src="a.png">`},

		// Ordered list start
		{input: `<ol start="10"><li>x</li></ol>`, expVal: `<ol start="10"><li>x</li></ol>`},
		{input: `<ol start="1; x"><li>x</li></ol>`, expVal: `<ol><li>x</li></ol>`},
		{input: `<ul start="10"><li>x</li></ul>`, expVal: `<ul><li>x</li></ul>`},

		// Input checkbox
		{input: `<input type="hidden">`, expVal: ``},
		{input: `<input type="checkbox">`, expVal: `<input type="checkbox">`},
		{input: `<input checked disabled autofocus>`, expVal: `<input checked="" disabled="">`},

		// Alerts
		{input: `<div class="markdown-alert markdown-alert-warning"><p class="markdown-alert-title">Warning</p></div>`, expVal: `<div class="markdown-alert markdown-alert-warning"><p class="markdown-alert-title">Warning</p></div>`},
		{input: `<div class="markdown-alert markdown-alert-warning ui modal"></div>`, expVal: `<div></div>`},
		{input: `<p class="footnotes"></p>`, expVal: `<p></p>`},

		// Footnotes
		{input: `<div class="footnotes" role="doc-endnotes"></div>`, expVal: `<div class="footnotes" role="doc-endnotes"></div>`},
		{input: `<a href="#user-content-fn:1" class="footnote-ref" role="doc-noteref">1</a>`, expVal: `<a href="#user-content-fn:1" class="footnote-ref" role="doc-noteref" rel="nofollow">1</a>`},
		{input: `<a href="#x" class="footnotes" role="button">1</a>`, expVal: `<a href="#x" rel="nofollow">1</a>`},

		// Data URIs: safe image types should be allowed
		{input: `<img src="data:image/png;base64,abc">`, expVal: `<img src="data:image/png;base64,abc">`},
		{input: `<img src="data:image/jpeg;base64,abc">`, expVal: `<img src="data:image/jpeg;base64,abc">`},
		{input: `<img src="data:image/gif;base64,abc">`, expVal: `<img src="data:image/gif;base64,abc">`},
		{input: `<img src="data:image/webp;base64,abc">`, expVal: `<img src="data:image/webp;base64,abc">`},

		// Data URIs: text/html must be stripped to prevent XSS (GHSA-xrcr-gmf5-2r8j)
		{input: `<a href="data:text/html;base64,PHNjcmlwdD5hbGVydCgnWFNTJyk8L3NjcmlwdD4=">Click</a>`, expVal: `Click`},
		{input: `<a href="data:text/html,<script>alert(1)</script>">XSS</a>`, expVal: `XSS`},
		{input: `<img src="data:text/html;base64,abc">`, expVal: ``},

		// Data URIs: SVG must be stripped (can contain embedded JavaScript)
		{input: `<img src="data:image/svg+xml;base64,abc">`, expVal: ``},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			assert.Equal(t, test.expVal, Sanitize(test.input))
			assert.Equal(t, test.expVal, string(SanitizeBytes([]byte(test.input))))
		})
	}
}

func TestSanitizer_XSS(t *testing.T) {
	NewSanitizer()
	tests := []struct {
		input  string
		expVal string
	}{
		// Scripts, styles and embedded content are removed with their content.
		{input: `<script>alert(1)</script>x`, expVal: `x`},
		{input: `<SCRIPT SRC=//xss.example/x.js></SCRIPT>x`, expVal: `x`},
		{input: `<iframe src="javascript:alert(1)"><b>x</b></iframe>y`, expVal: `y`},
		{input: `<object data="x"><b>x</b></object>y`, expVal: `y`},
		{input: `<style>body{background:url(javascript:alert(1))}</style>y`, expVal: `y`},
		{input: `<title><script>alert(1)</script></title>x`, expVal: `x`},
		{input: `<svg/onload=alert(1)>x`, expVal: `x`},
		{input: `<math><mi xlink:href="javascript:alert(1)">x</mi></math>`, expVal: `x`},
		{input: `<base href="javascript:alert(1)//"><link rel="stylesheet" href="x"><meta http-equiv="refresh" content="0;url=javascript:alert(1)">`, expVal: ``},
		{input: `<form action="x"><input type="submit" formaction="javascript:alert(1)"><button>b</button></form>`, expVal: `b`},

		// Event handlers, styles and classes are removed.
		{input: `<img src=x onerror=alert(1)>`, expVal: `<img src="x">`},
		{input: `<div style="background:url(javascript:alert(1))" onclick="x" class="y">z</div>`, expVal: `<div>z</div>`},
		{input: `<p title="x" onmouseover="alert(1)">y</p>`, expVal: `<p title="x">y</p>`},

		// Disallowed schemes, also obfuscated with character references,
		// control characters, white space and mixed case.
		{input: `<a href="JaVaScRiPt:alert(1)">x</a>`, expVal: `x`},
		{input: `<a href="vbscript:msgbox(1)">x</a>`, expVal: `x`},
		{input: `<a href=" &#14;  javascript:alert(1)">x</a>`, expVal: `x`},
		{input: "<a href=\"java\x00script:alert(1)\">x</a>", expVal: `x`},
		{input: `<IMG SRC="jav&#x09;ascript:alert('XSS');">`, expVal: ``},
		{input: `<IMG SRC=&#106;&#97;&#118;&#97;&#115;&#99;&#114;&#105;&#112;&#116;&#58;&#97;&#108;&#101;&#114;&#116;&#40;&#39;&#88;&#83;&#83;&#39;&#41;>`, expVal: ``},
		{input: `<IMG SRC=&#x6A&#x61&#x76&#x61&#x73&#x63&#x72&#x69&#x70&#x74&#x3A&#x61&#x6C&#x65&#x72&#x74&#x28&#x27&#x58&#x53&#x53&#x27&#x29>`, expVal: ``},
		{input: `<a href="data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==">x</a>`, expVal: `x`},
		{input: `<a href="http://a/b c">x</a>`, expVal: `x`},

		// Broken and nested markup never produces a tag.
		{input: `<<script>script>alert(1)<</script>/script>`, expVal: `&lt;/script&gt;`},
		{input: `<scr<script>ipt>alert(1)</script>`, expVal: `ipt&gt;alert(1)`},
		{input: `<textarea><script>alert(1)</script></textarea>`, expVal: `&lt;script&gt;alert(1)&lt;/script&gt;`},
		{input: `<noscript><p title="</noscript><img src=x onerror=alert(1)>">`, expVal: `<img src="x">&#34;&gt;`},
		{input: `<!--<script>alert(1)</script>-->x`, expVal: `x`},
		{input: `<!DOCTYPE html>x`, expVal: `x`},

		// Attribute values are always quoted and escaped.
		{input: `<b title="a&quot; onmouseover=&quot;alert(1)">x</b>`, expVal: `<b>x</b>`},
		{input: `<a href="/a?b=&quot;&gt;">x</a>`, expVal: `<a href="/a?b=&#34;&gt;" rel="nofollow">x</a>`},
		{input: `<p>&lt;script&gt; &amp; &quot; ' &#39;</p>`, expVal: `<p>&lt;script&gt; &amp; &#34; &#39; &#39;</p>`},
		{input: `a < b > c & d`, expVal: `a &lt; b &gt; c &amp; d`},

		// Links get rel="nofollow" and lose their targets.
		{input: `<a href="https://example.com" target="_blank" rel="opener">x</a>`, expVal: `<a href="https://example.com" rel="nofollow">x</a>`},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			assert.Equal(t, test.expVal, Sanitize(test.input))
			assert.Equal(t, test.expVal, string(SanitizeBytes([]byte(test.input))))
		})
	}
}

func TestSanitizer_AllowedAttributes(t *testing.T) {
	NewSanitizer()
	tests := []struct {
		input  string
		expVal string
	}{
		{input: `<a href="/user/repo/src/main/README.md" id="x" title="t">l</a>`, expVal: `<a href="/user/repo/src/main/README.md" id="x" title="t" rel="nofollow">l</a>`},
		{input: `<a href="mailto:a@example.com">m</a>`, expVal: `<a href="mailto:a@example.com" rel="nofollow">m</a>`},
		{input: `<a href="#user-content-x">h</a>`, expVal: `<a href="#user-content-x" rel="nofollow">h</a>`},
		{input: `<a href="x" rel="nofollow">x</a>`, expVal: `<a href="x" rel="nofollow">x</a>`},
		{input: `<img src="https://example.com/a.png" alt="A b" title="T" width="10" height="20%" align="middle" usemap="#m">`, expVal: `<img src="https://example.com/a.png" alt="A b" title="T" width="10" height="20%" align="middle" usemap="#m">`},
		{input: `<blockquote cite="https://example.com">q</blockquote>`, expVal: `<blockquote cite="https://example.com">q</blockquote>`},
		{input: `<td align="center" valign="top" colspan="2" rowspan="3" nowrap="" headers="a b" scope="row" abbr="x">c</td>`, expVal: `<td align="center" valign="top" colspan="2" rowspan="3" nowrap="" headers="a b" scope="row" abbr="x">c</td>`},
		{input: `<details open><summary>s</summary>d</details>`, expVal: `<details open=""><summary>s</summary>d</details>`},
		{input: `<span dir="rtl" lang="ar" id="a:b">x</span><bdo dir="ltr">y</bdo>`, expVal: `<span dir="rtl" lang="ar" id="a:b">x</span><bdo dir="ltr">y</bdo>`},
		{input: `<ol type="a" start="3"><li value="5">x</li></ol>`, expVal: `<ol type="a" start="3"><li value="5">x</li></ol>`},
		{input: `<time datetime="2024-01-02T03:04:05Z">t</time><del datetime="2024-01-02" cite="x">d</del>`, expVal: `<time datetime="2024-01-02T03:04:05Z">t</time><del datetime="2024-01-02" cite="x">d</del>`},
		{input: `<meter value="0.5" min="0" max="1">m</meter><progress value="1" max="2">p</progress>`, expVal: `<meter value="0.5" min="0" max="1">m</meter><progress value="1" max="2">p</progress>`},
		{input: `<area href="/a" alt="x" shape="rect" coords="0,0,1,1" rel="author">`, expVal: `<area href="/a" alt="x" shape="rect" coords="0,0,1,1" rel="author nofollow">`},
		{input: `<li class="task-list-item" data-source-lines="3-4"><input type="checkbox" checked disabled>x</li>`, expVal: `<li class="task-list-item" data-source-lines="3-4"><input type="checkbox" checked="" disabled="">x</li>`},

		// Values are matched as regular expressions with the (?i) flag would,
		// which fold "ſ" (U+017F) with "s".
		{input: `<td align="JUſTIFY">x</td>`, expVal: `<td align="JUſTIFY">x</td>`},

		// Elements that need attributes are removed with their end tags.
		{input: `<a>x</a><a href="#">y</a><img>z`, expVal: `xyz`},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			assert.Equal(t, test.expVal, Sanitize(test.input))
			assert.Equal(t, test.expVal, string(SanitizeBytes([]byte(test.input))))
		})
	}
}

func TestSanitizer_Blank(t *testing.T) {
	for _, input := range []string{"", " \n\t", "\u00a0"} {
		assert.Equal(t, input, Sanitize(input))
		assert.Equal(t, []byte(input), SanitizeBytes([]byte(input)))
	}
}
