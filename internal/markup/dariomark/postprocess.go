package dariomark

import (
	"bytes"
	"io"
	"strings"

	"golang.org/x/net/html"
)

// postProcess applies Options.PostProcess to the HTML of a document with raw
// HTML. It tokenizes the whole output because raw HTML can change how the
// HTML that follows it is tokenized.
//
// Text outside links, code, preformatted text and raw text elements gets
// special links, images are wrapped in links to themselves, and end tags are
// renamed to close the innermost open element. If the tokenizer fails other
// than at the end of the input, the input is returned unchanged.
func (r *renderer) postProcess(b []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(b) + len(b)/8)
	var startTags []string
	rawText := false
	z := html.NewTokenizer(bytes.NewReader(b))
	for z.Next() != html.ErrorToken {
		t := z.Token()
		inRawText := rawText
		rawText = false
		switch t.Type {
		case html.TextToken:
			if inRawText {
				out.WriteString(t.String())
				break
			}
			out.Write(r.specialLinks([]byte(t.String())))
		case html.StartTagToken:
			rawText = isRawTextElement(t.Data)
			if t.Data == "img" {
				if !r.imageLink(&out, t) {
					out.WriteString(t.String())
				}
				continue
			}
			out.WriteString(t.String())
			if t.Data == "a" || t.Data == "code" || t.Data == "pre" {
				// Copy the element verbatim, up to the end tag with its name
				// that closes as many elements as were opened.
				depth := 1
				for z.Next() != html.ErrorToken {
					inner := z.Token()
					out.WriteString(inner.String())
					if inner.Type == html.StartTagToken && !isNoEndTag(inner.Data) {
						depth++
					}
					if inner.Type == html.EndTagToken {
						depth--
						if depth <= 0 && inner.Data == t.Data {
							break
						}
					}
				}
				continue
			}
			if !isNoEndTag(t.Data) {
				startTags = append(startTags, t.Data)
			}
		case html.SelfClosingTagToken:
			// The tokenizer reads the content after "<textarea/>" as raw
			// text too.
			rawText = isRawTextElement(t.Data)
			out.WriteString(t.String())
		case html.EndTagToken:
			if len(startTags) == 0 {
				out.WriteString(t.String())
				break
			}
			out.WriteString("</")
			out.WriteString(startTags[len(startTags)-1])
			out.WriteByte('>')
			startTags = startTags[:len(startTags)-1]
		default:
			out.WriteString(t.String())
		}
	}
	if z.Err() != io.EOF {
		return b
	}
	return out.Bytes()
}

// isNoEndTag reports whether postProcess treats name as an element without
// an end tag.
func isNoEndTag(name string) bool {
	switch name {
	case "input", "br", "hr", "img":
		return true
	}
	return false
}

// isRawTextElement reports whether the tokenizer reads the content of the
// element name as one text token, e.g., "script" and "textarea". The
// sanitizer drops or escapes that content, so links in it would show as text.
func isRawTextElement(name string) bool {
	switch name {
	case "iframe", "noembed", "noframes", "noscript", "plaintext", "script", "style", "textarea", "title", "xmp":
		return true
	}
	return false
}

func (r *renderer) specialLinks(text []byte) []byte {
	if f := r.opts.SpecialLinks; f != nil {
		if links := f(text); len(links) > 0 {
			return AppendSpecialLinks(nil, text, links)
		}
	}
	return text
}

// imageLink writes an image wrapped in a link to its source and reports
// whether it did. Relative sources are resolved against the raw file URL of
// Options.LinkBase. Images without a source or with a data URI are not
// wrapped.
func (r *renderer) imageLink(out *bytes.Buffer, t html.Token) bool {
	var src, alt string
	for _, a := range t.Attr {
		switch a.Key {
		case "src":
			src = a.Val
		case "alt":
			alt = a.Val
		}
	}
	if src == "" || strings.HasPrefix(src, "data:") {
		return false
	}
	prefix := r.opts.LinkBase
	relative := r.opts.IsLink == nil || !r.opts.IsLink([]byte(src))
	if relative {
		prefix = strings.Replace(prefix, "/src/", "/raw/", 1)
		if src[0] != '/' {
			prefix += "/"
		}
	}
	out.WriteString(`<a href="`)
	if relative {
		out.WriteString(html.EscapeString(prefix))
	}
	out.WriteString(html.EscapeString(src))
	out.WriteString(`">`)
	if relative {
		out.WriteString(`<img src="`)
		out.WriteString(html.EscapeString(strings.ReplaceAll(prefix+src, " ", "%20")))
		out.WriteByte('"')
		if alt != "" {
			out.WriteString(` alt="`)
			out.WriteString(html.EscapeString(alt))
			out.WriteByte('"')
		}
		out.WriteByte('>')
	} else {
		out.WriteString(t.String())
	}
	out.WriteString("</a>")
	return true
}
