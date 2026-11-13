// Package dariomark renders Markdown to HTML. Dariomark implements CommonMark
// 0.31.2 and the GitHub Flavored Markdown extensions (tables, strikethrough,
// task lists and extended autolinks), plus footnotes, GitHub alerts and the
// Gogs-specific link handling. It is safe for concurrent use and runs in
// linear time.
package dariomark

import (
	"bytes"
	"sync"
)

// Typographer configures the replacement of ASCII punctuation with
// typographic punctuation.
type Typographer struct {
	Enabled bool
	// Dashes replaces "--" and "---" with dashes.
	Dashes bool
	// LatexDashes renders "--" as an en dash and "---" as an em dash. Otherwise
	// "--" is an em dash.
	LatexDashes bool
	// AngledQuotes renders double quotes as angled quotes.
	AngledQuotes bool
	// Fractions renders fractions like "2/3" as a superscript numerator and a
	// subscript denominator. Otherwise only "1/2", "1/4" and "3/4" are
	// replaced, with "½", "¼" and "¾".
	Fractions bool
}

// Options configures rendering. The zero value renders CommonMark.
type Options struct {
	Tables        bool
	Strikethrough bool
	TaskLists     bool
	// Linkify detects URLs, "www." links and email addresses in text.
	Linkify bool
	// LinkifyURL returns the length of the URL at the start of b, or 0 if there
	// is none. It is only called on input that starts with "http://",
	// "https://" or "ftp://". When nil, URLs are detected as in GFM: the
	// scheme, a valid domain, and everything up to whitespace or "<".
	LinkifyURL func(b []byte) int
	// LinkifyAfterPunct also detects URLs whose scheme directly follows a
	// character other than an ASCII letter or digit.
	LinkifyAfterPunct bool
	Footnotes         bool
	FootnoteIDPrefix  string
	// Alerts renders GitHub alerts, e.g., "> [!NOTE]".
	Alerts bool
	// HeadingIDs gives headings an id attribute made of HeadingIDPrefix and
	// a slug of their text, as GitHub does, and lists them in
	// Result.Headings.
	HeadingIDs      bool
	HeadingIDPrefix string
	// HardWraps renders soft line breaks as hard line breaks.
	HardWraps bool
	// XHTML renders void elements like "<br />".
	XHTML       bool
	Typographer Typographer

	// LinkBase, when RelativeLinks is true, is joined with the destinations of
	// links that are not absolute and do not start with "#".
	RelativeLinks bool
	LinkBase      string
	// IsLink reports whether a link destination is absolute.
	IsLink func(dest []byte) bool
	// TextAutoLinksInRawAnchors renders autolinks inside raw HTML anchors,
	// e.g., "<a href="x">http://example.com</a>", as text.
	TextAutoLinksInRawAnchors bool
	// ShortenAutoLink, if not nil, may render an autolink to a URL as the
	// returned HTML instead.
	ShortenAutoLink func(url []byte) ([]byte, bool)

	// Gogs post-processing and sanitizing. See Render.
	Gogs *GogsOptions
}

// GogsOptions configures the post-processing that turns special references
// into links and wraps images in links, and the sanitizing of documents
// without raw HTML.
type GogsOptions struct {
	// URLPrefix is the prefix of relative image sources.
	URLPrefix string
	// SpecialLinks returns the mentions, issue references and commit SHAs in
	// the HTML-escaped text that render as links, in order and without
	// overlaps.
	//
	// In documents without raw HTML, text longer than 32 KB is passed in
	// pieces. A piece ends after a space that no "http" precedes on its line,
	// or after a line ending followed by a character other than an ASCII
	// letter or digit, "-", "_", "." or "#". SpecialLinks must render the
	// pieces as it would the whole text: references other than URLs do not
	// contain spaces, and the start of the text counts as a space.
	SpecialLinks func(text []byte) []SpecialLink
	// Sanitizer, if not nil, enables sanitizing documents without raw HTML.
	Sanitizer *Sanitizer
}

// SpecialLink is a reference in text that renders as a link, see
// GogsOptions.SpecialLinks.
type SpecialLink struct {
	// Start and End are the offsets of the reference in the text.
	Start, End int
	// Href is the HTML-escaped destination of the link.
	Href []byte
	// Text is the HTML-escaped text of the link.
	Text []byte
	// Code reports whether the text of the link is code, e.g., a commit SHA.
	Code bool
}

// AppendSpecialLinks appends text to dst with links in place of the
// references, as HTML.
func AppendSpecialLinks(dst, text []byte, links []SpecialLink) []byte {
	last := 0
	for _, l := range links {
		dst = append(dst, text[last:l.Start]...)
		dst = append(dst, `<a href="`...)
		dst = append(dst, l.Href...)
		dst = append(dst, `">`...)
		if l.Code {
			dst = append(dst, "<code>"...)
		}
		dst = append(dst, l.Text...)
		if l.Code {
			dst = append(dst, "</code>"...)
		}
		dst = append(dst, "</a>"...)
		last = l.End
	}
	return append(dst, text[last:]...)
}

// Result is the output of Render.
type Result struct {
	HTML []byte
	// Sanitized reports whether HTML has been sanitized, which happens for
	// documents without raw HTML when Options.Gogs is set.
	Sanitized bool
	// Headings are the headings of the document in order, when
	// Options.HeadingIDs is set. They are the entries of a table of contents.
	Headings []Heading
}

// Heading is a heading of a rendered document.
type Heading struct {
	Level int
	// Text is the text content of the heading.
	Text string
	// ID is the id attribute of the heading. It is empty when the text has no
	// letters or digits, in which case the heading has no id.
	ID string
}

var parserPool = sync.Pool{New: func() any { return new(parser) }}

// Render renders the Markdown source to HTML.
//
// Without Options.Gogs, raw HTML is passed through, and the output is not
// sanitized.
//
// With Options.Gogs, the output is post-processed: special references in text
// become links, standalone images are wrapped in links to themselves, and
// links, code and preformatted text are left alone. Documents without raw
// HTML are sanitized while rendering, and Result.Sanitized is true. Other
// documents must be sanitized by the caller.
func Render(src []byte, opts *Options) Result {
	p := parserPool.Get().(*parser)
	defer func() {
		p.reset()
		parserPool.Put(p)
	}()
	p.opts = opts
	if len(src) >= streamMinSize && !bytes.Contains(src, []byte("]:")) {
		return p.renderStream(src)
	}
	doc := p.parse(src, nil)

	r := &p.r
	r.init(p, opts)
	if opts.Gogs == nil {
		r.renderHTML(doc)
		return Result{HTML: r.out.Bytes(), Headings: r.headingList()}
	}
	if !p.hasRawHTML {
		// Inline raw HTML is only found while rendering, which then starts
		// over without sanitizing.
		refBudget, padBudget := p.refBudget, p.padBudget
		if out, ok := r.renderSanitized(func() { r.renderDocument(doc) }); ok {
			return Result{HTML: out, Sanitized: true, Headings: r.headingList()}
		}
		p.refBudget, p.padBudget = refBudget, padBudget
		p.footnotes.resetInline()
		r.init(p, opts)
	}
	r.renderHTML(doc)
	return Result{HTML: r.postProcess(r.out.Bytes()), Headings: r.headingList()}
}

// streamMinSize is the size from which documents are rendered while they are
// parsed, when they allow it, see parser.renderStream.
var streamMinSize = 256 << 10

// renderStream renders src while parsing it: each top-level block is
// rendered and freed when it is closed, which keeps memory small for large
// documents. Link reference and footnote definitions change how the blocks
// before them render, so src must have none, i.e., no "]:".
func (p *parser) renderStream(src []byte) Result {
	opts := p.opts
	r := &p.r
	r.init(p, opts)
	if opts.Gogs != nil {
		// Raw HTML stops the sanitizing render, which then starts over.
		if out, ok := r.renderSanitized(func() {
			p.parse(src, r.renderClosed)
			if p.hasRawHTML {
				r.stop = true
			}
		}); ok {
			return Result{HTML: out, Sanitized: true, Headings: r.headingList()}
		}
		p.reset()
		p.opts = opts
		r.init(p, opts)
	}
	r.out.Grow(len(src) + len(src)/4)
	r.s = &r.html
	p.parse(src, r.renderClosed)
	if opts.Gogs == nil {
		return Result{HTML: r.out.Bytes(), Headings: r.headingList()}
	}
	return Result{HTML: r.postProcess(r.out.Bytes()), Headings: r.headingList()}
}

// HTML renders the Markdown source to HTML without post-processing.
func HTML(src []byte, opts *Options) []byte {
	o := *opts
	o.Gogs = nil
	return Render(src, &o).HTML
}

// AppendEscaped appends b to dst with "&", "<", ">" and '"' escaped.
func AppendEscaped(dst, b []byte) []byte {
	for _, c := range b {
		switch c {
		case '&':
			dst = append(dst, "&amp;"...)
		case '<':
			dst = append(dst, "&lt;"...)
		case '>':
			dst = append(dst, "&gt;"...)
		case '"':
			dst = append(dst, "&quot;"...)
		default:
			dst = append(dst, c)
		}
	}
	return dst
}
