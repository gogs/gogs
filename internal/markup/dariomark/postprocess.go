package dariomark

import (
	"bytes"
	"io"
	"slices"
	"strings"

	"golang.org/x/net/html"
)

// postProcess applies Options.PostProcess to the HTML of a document with raw
// HTML. It tokenizes the whole output because raw HTML can change how the
// HTML that follows it is tokenized.
//
// Text outside links, code, preformatted text and raw text elements gets
// special links, and text outside code and preformatted text gets emoji.
// Images are wrapped in links to themselves, except in links, where only their
// sources are resolved, and end tags are renamed to close the innermost open
// element. If the tokenizer fails other than at the end of the input, the
// input is returned unchanged.
func (r *renderer) postProcess(b []byte) []byte {
	var out outBuf
	out.Grow(len(b) + len(b)/8)
	// Front matter is text, which is not linked.
	out.Write(b[:r.frontMatterSize])
	b = b[r.frontMatterSize:]
	var startTags []string
	rawText := false
	emoji := emojiText{names: r.opts.Emoji}
	// inText reports whether the last tokens are text, possibly followed by
	// end tags without a start tag. HTML parsers ignore those end tags, so
	// the text around them is one text node.
	inText := false
	z := html.NewTokenizer(bytes.NewReader(b))
	for z.Next() != html.ErrorToken {
		t := z.Token()
		inRawText := rawText
		rawText = false
		continuesText := inText
		inText = false
		switch t.Type {
		case html.TextToken:
			if inRawText {
				out.WriteString(t.String())
				break
			}
			if !continuesText {
				emoji.startNode()
			}
			r.specialLinks(&out, []byte(t.String()), &emoji)
			inText = true
		case html.StartTagToken, html.SelfClosingTagToken:
			// The tokenizer reads the content after "<textarea/>" as raw
			// text too.
			rawText = isRawTextElement(t.Data)
			if t.Data == "img" {
				if !r.imageLink(&out, t) {
					out.WriteString(t.String())
				}
				continue
			}
			out.WriteString(t.String())
			if t.Type == html.SelfClosingTagToken {
				continue
			}
			if t.Data == "a" || t.Data == "code" || t.Data == "pre" {
				// Copy the element verbatim, up to the end tag with its name
				// that closes as many elements as were opened. Text in links
				// gets emoji.
				depth := 1
				code := 0
				if t.Data != "a" {
					code = 1
				}
				for z.Next() != html.ErrorToken {
					inner := z.Token()
					if inner.Type == html.TextToken && code == 0 {
						emoji.startNode()
						emoji.write(&out, []byte(inner.String()))
						continue
					}
					if (inner.Type == html.StartTagToken || inner.Type == html.SelfClosingTagToken) && inner.Data == "img" && code == 0 {
						inner.Attr = r.linkedImageAttrs(inner.Attr)
					}
					out.WriteString(inner.String())
					if inner.Type == html.StartTagToken && !isNoEndTag(inner.Data) {
						depth++
						if inner.Data == "code" || inner.Data == "pre" {
							code++
						}
					}
					if inner.Type == html.EndTagToken {
						depth--
						if inner.Data == "code" || inner.Data == "pre" {
							code--
						}
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
		case html.EndTagToken:
			inText = continuesText && !slices.Contains(startTags, t.Data)
			if len(startTags) == 0 {
				out.WriteString(t.String())
				break
			}
			out.WriteString("</")
			out.WriteString(startTags[len(startTags)-1])
			out.writeByte('>')
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

// specialLinks writes text, the escaped text of a text node, to out with
// special references rendered as links, and emoji shortcodes replaced by e.
// The links split the text into text nodes.
func (r *renderer) specialLinks(out *outBuf, text []byte, e *emojiText) {
	f := r.opts.SpecialLinks
	if f == nil {
		e.write(out, text)
		return
	}
	links := f(text)
	if len(links) == 0 {
		e.write(out, text)
		return
	}
	last := 0
	for _, l := range links {
		e.write(out, text[last:l.Start])
		e.startNode()
		out.Write(appendSpecialLink(out.AvailableBuffer(), l))
		last = l.End
	}
	e.write(out, text[last:])
}

// rawImagePrefix returns the prefix that resolves the relative image source
// src against the raw file URL of Options.LinkBase, and reports whether src is
// relative. Sources that are empty or data URIs are not.
func (r *renderer) rawImagePrefix(src string) (string, bool) {
	if src == "" || strings.HasPrefix(src, "data:") || r.opts.IsLink != nil && r.opts.IsLink([]byte(src)) {
		return "", false
	}
	prefix := strings.Replace(r.opts.LinkBase, "/src/", "/raw/", 1)
	if src[0] != '/' {
		prefix += "/"
	}
	return prefix, true
}

// imageLink writes an image wrapped in a link to its source and reports
// whether it did. Relative sources are resolved against the raw file URL of
// Options.LinkBase. Images without a source or with a data URI are not
// wrapped.
func (r *renderer) imageLink(out *outBuf, t html.Token) bool {
	var src string
	for _, a := range t.Attr {
		if a.Key == "src" {
			src = a.Val
		}
	}
	if src == "" || strings.HasPrefix(src, "data:") {
		return false
	}
	prefix, relative := r.rawImagePrefix(src)
	out.WriteString(`<a href="`)
	if relative {
		out.WriteString(html.EscapeString(prefix))
	}
	out.WriteString(html.EscapeString(src))
	out.WriteString(`">`)
	if relative {
		// Keep the other attributes, e.g., width and align in READMEs.
		t.Attr = slices.Clone(t.Attr)
		for i := range t.Attr {
			if t.Attr[i].Key == "src" {
				t.Attr[i].Val = strings.ReplaceAll(prefix+src, " ", "%20")
			}
		}
	}
	out.WriteString(t.String())
	out.WriteString("</a>")
	return true
}

// linkedImageAttrs returns the attributes of an image inside a link with a
// relative source resolved like imageLink does. The image is not wrapped in
// another link, so that it keeps going where the author's link goes.
func (r *renderer) linkedImageAttrs(attrs []html.Attribute) []html.Attribute {
	for i, a := range attrs {
		if a.Key != "src" {
			continue
		}
		if prefix, ok := r.rawImagePrefix(a.Val); ok {
			attrs = slices.Clone(attrs)
			attrs[i].Val = strings.ReplaceAll(prefix+a.Val, " ", "%20")
		}
		break
	}
	return attrs
}
