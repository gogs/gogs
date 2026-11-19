package dariomark

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

var tokenizerInputs = []string{
	``,
	`plain text`,
	`<p class="a" id='b' title=c d>x</p>`,
	`<br/><br /><p a=/>x<p a="/"/>`,
	`<A HREF="x" href="y" Href=z>t</A>`,
	`<div =a b= =c d=>`,
	"<p\x00 a\x00=\"\x00\">\x00</p\x00>",
	"a\r\nb\rc<p title=\"x\r\ny\">",
	`&amp; &amp &notit; &#65; &#x41 &#; &#xZZ; &# &lt&gt <p title="&copy=x &copy;=y &copyx">`,
	`<!-- a --><!--> <!---> <!-- -- --!> <!-x> <!DOCTYPE html> <!doctype> <? pi ?> </ x> </> </3>`,
	`<!--`, `<!-`, `<!`, `<!DOC`, `<!DOCTYPE`, `<`, `</`, `</a`, `<a`, `<a b`, `<a b=`, `<a b="`, `<a b='x`,
	`<script>a<b>c</b></script>d`,
	`<script><!--<script></script>--></script>x`,
	`<script><!-- </script> -->x`,
	`<SCRIPT>x</ScRiPt >y`,
	`<style>a</style b>c`,
	`<textarea>&amp;<b></textarea>`,
	`<title>&lt;x</title>`,
	`<xmp>&amp;<b></xmp>`,
	`<iframe><b>x</iframe>`,
	`<noscript><b></noscript><noembed>x</noembed><noframes>y</noframes>`,
	`<plaintext><b>&amp;</b>`,
	`<script/>x</script>y`,
	`<a b c d e f g h i j k l m n o p q r s t u v w x y z A B C a b>`,
	`<x-y:z.w>`,
	"<p\tid=a\fclass=b\n>",
}

// prefixedTokenizerInputs are checked with every prefix, so that the input
// ends in each state of the tokenizer, e.g., of script data.
var prefixedTokenizerInputs = []string{
	`<script>a<b<!c<!-d<!--e-f-<g--h--<i--->j<!--<k</l</scriptx<script>m-n-<o--p--<q---r--->s<t</u</script>v-->w</script >x`,
	`<script><!--<SCRIPT/>-</script>a</script>b`,
	`<script><!--<1-->a</script>b`,
	`<textarea>a<b</c</textareax</TEXTAREA b>c`,
	`<title>a</title/>b`,
	`<!--a--!b--!>c<!--d-!>e-->f<!>g<!x>`,
	`<a b = "c" d='e' f=g h=/ i/>`,
	`a&#1;b&#x1;c&d;`,
}

// TestHTMLTokenizer checks that htmlTokenizer splits HTML into the tokens of
// html.Tokenizer, and that appendDecoded decodes them the same.
func TestHTMLTokenizer(t *testing.T) {
	for _, s := range tokenizerInputs {
		checkTokenizer(t, []byte(s))
	}
	for _, s := range prefixedTokenizerInputs {
		for i := 0; i <= len(s); i++ {
			checkTokenizer(t, []byte(s[:i]))
		}
	}
}

func FuzzHTMLTokenizer(f *testing.F) {
	for _, s := range tokenizerInputs {
		f.Add(s)
	}
	for _, s := range prefixedTokenizerInputs {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		checkTokenizer(t, []byte(s))
	})
}

func checkTokenizer(t *testing.T, b []byte) {
	t.Helper()
	want := html.NewTokenizer(bytes.NewReader(b))
	var got htmlTokenizer
	got.reset(b)
	for i := 0; ; i++ {
		wt := want.Next()
		gt := got.next()
		require.Equal(t, wt, gt, "type of token %d of %q", i, b)
		if wt == html.ErrorToken {
			return
		}
		require.Equal(t, string(want.Raw()), string(b[got.raw.start:got.raw.end]), "raw token %d of %q", i, b)
		tok := want.Token()
		data := b[got.data.start:got.data.end]
		switch wt {
		case html.TextToken:
			text := appendDecoded(nil, data, got.convertNUL, !got.textIsRaw, false)
			require.Equal(t, tok.Data, string(text), "text of token %d of %q", i, b)
		case html.CommentToken, html.DoctypeToken:
			// The sanitizer drops them, so only their raw bytes matter.
		default:
			name := string(lowerASCII(bytes.ReplaceAll(data, []byte{0}, []byte("\ufffd"))))
			require.Equal(t, tok.Data, name, "name of token %d of %q", i, b)
			if wt == html.EndTagToken {
				break
			}
			require.Len(t, got.attr, len(tok.Attr), "attributes of token %d of %q", i, b)
			for j, a := range got.attr {
				key := string(lowerASCII(bytes.ReplaceAll(b[a[0].start:a[0].end], []byte{0}, []byte("\ufffd"))))
				val := string(appendDecoded(nil, b[a[1].start:a[1].end], true, true, true))
				require.Equal(t, tok.Attr[j].Key, key, "attribute %d of token %d of %q", j, i, b)
				require.Equal(t, tok.Attr[j].Val, val, "attribute %d of token %d of %q", j, i, b)
			}
		}
	}
}
