// Package dariomark renders Markdown to HTML. dariomark implements CommonMark
// 0.31.2 and the GitHub Flavored Markdown extensions (tables, strikethrough,
// task lists and extended autolinks), plus footnotes, GitHub alerts and the
// Gogs-specific link handling. It is safe for concurrent use and runs in
// linear time.
package dariomark

import (
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
}

// Options configures rendering. The zero value renders CommonMark.
type Options struct {
	Tables        bool
	Strikethrough bool
	TaskLists     bool
	// Linkify detects URLs, "www." links and email addresses in text.
	Linkify bool
	// LinkifyURL returns the length of the URL at the start of b, or 0 if there
	// is none. It is required when Linkify is enabled, and only called on input
	// that starts with "http://", "https://" or "ftp://".
	LinkifyURL func(b []byte) int
	// LinkifyAfterPunct also detects URLs whose scheme directly follows a
	// character other than an ASCII letter or digit.
	LinkifyAfterPunct bool
	Footnotes         bool
	FootnoteIDPrefix  string
	// Alerts renders GitHub alerts, e.g., "> [!NOTE]".
	Alerts bool
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
	// SpecialLinks renders mentions, issue references and commit SHAs in the
	// HTML-escaped text to links. It returns text unchanged when there is
	// nothing to render.
	SpecialLinks func(text []byte) []byte
	// Sanitizer, if not nil, enables sanitizing documents without raw HTML.
	Sanitizer *Sanitizer
}

// Result is the output of Render.
type Result struct {
	HTML []byte
	// Sanitized reports whether HTML has been sanitized, which happens for
	// documents without raw HTML when Options.Gogs is set.
	Sanitized bool
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
	doc := p.parse(src)

	var r renderer
	r.init(p, opts)
	if opts.Gogs == nil {
		r.renderHTML(doc)
		return Result{HTML: r.out.Bytes()}
	}
	if !p.hasRawHTML {
		if out, ok := r.renderSanitized(doc); ok {
			return Result{HTML: out, Sanitized: true}
		}
		r.init(p, opts)
	}
	r.renderHTML(doc)
	return Result{HTML: r.postProcess(r.out.Bytes())}
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
