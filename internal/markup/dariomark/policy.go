package dariomark

import (
	"bytes"
	"net/url"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"

	"gogs.io/gogs/internal/lazyregexp"
)

// isSafeDataURI reports whether the data URI u uses a safe image MIME type.
func isSafeDataURI(u *url.URL) bool {
	// The opaque data of a data URI has the form "mediatype;base64,data" or
	// "mediatype,data". Only common image MIME types are allowed.
	mediatype, _, _ := strings.Cut(u.Opaque, ";")
	mediatype, _, _ = strings.Cut(mediatype, ",")
	switch strings.TrimSpace(strings.ToLower(mediatype)) {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/x-icon":
		return true
	}
	return false
}

// Policy is an allowlist of elements and attributes for sanitizing HTML of
// user content. Everything else is dropped: tags not allowed are removed,
// keeping their content unless they are in skipContent, and attributes not
// allowed are removed. Elements that must have attributes, e.g., links, are
// removed when none are left.
//
// It reproduces, byte for byte, the output of the UGC policy of bluemonday
// v1.0.27, which it replaces, so that stored and cached HTML does not
// change. The only exception is data URIs, which are always checked, even
// when the custom URL schemes list "data".
//
// A Policy is safe for concurrent use.
type Policy struct {
	elements map[string]*elementPolicy
	// schemes are the URL schemes allowed without checks. Data URIs are
	// allowed if dataURIs and isSafeDataURI, and relative URLs always.
	schemes  map[string]bool
	dataURIs bool
}

type elementPolicy struct {
	name string
	// bare is whether the element is kept without attributes.
	bare  bool
	attrs map[string]*attrPolicy
	// link is whether the element gets rel="nofollow" when it has an href.
	link bool
}

type attrPolicy struct {
	name string
	// check reports whether a value is allowed. nil allows any value.
	check func(v []byte) bool
	// url is whether the value must be a URL that validURL allows, which
	// replaces it with its normalized form.
	url bool
}

// skipContent are the elements whose content is removed with them.
var skipContent = map[string]bool{
	"frame": true, "frameset": true, "iframe": true, "noembed": true, "noframes": true, "noscript": true,
	"nostyle": true, "object": true, "script": true, "style": true, "title": true,
}

// bareElements are the elements that are kept without attributes, if
// allowed.
var bareElements = strings.Fields(`abbr acronym address article aside audio b bdi blockquote body br
	button canvas caption center cite code col colgroup datalist dd del details dfn div dl dt em
	fieldset figcaption figure footer h1 h2 h3 h4 h5 h6 head header hgroup hr html i ins kbd li mark
	marquee nav ol optgroup option p picture pre q rp rt ruby s samp script section select small span
	strike strong style sub summary sup svg table tbody td textarea tfoot th thead title time tr tt u
	ul var video wbr`)

// NewPolicy returns the UGC policy of bluemonday, extended with the rules for
// the HTML that Render produces and the custom URL schemes if markdown.
func NewPolicy(markdown bool, customSchemes []string) *Policy {
	p := &Policy{
		elements: map[string]*elementPolicy{},
		schemes:  map[string]bool{"mailto": true, "http": true, "https": true},
	}

	p.allow(nil, "", "article aside figure section summary h1 h2 h3 h4 h5 h6 hgroup br div hr p span wbr")
	p.allow(isOpen, "open", "details")
	p.allow(nil, "cite", "blockquote q")
	p.allow(nil, "href", "a area")
	p.allow(mapNamePattern.Match, "name", "map")
	p.allow(isText, "alt", "area img")
	p.allow(coordsPattern.Match, "coords", "area")
	p.allow(spaceSeparatedTokensPattern.Match, "rel", "area")
	p.allow(shapePattern.Match, "shape", "area")
	p.allow(usemapPattern.Match, "usemap", "img")
	p.allow(nil, "", "abbr acronym cite code dfn em figcaption mark s samp strong sub sup var")
	p.allow(iso8601Pattern.Match, "datetime", "time del ins")
	p.allow(nil, "", "b i pre small strike tt u")
	p.allow(isDirection, "dir", "bdi bdo")
	p.allow(nil, "", "rp rt ruby")
	p.allow(isText, "cite", "del ins")
	p.allow(listTypePattern.Match, "type", "ol ul li")
	p.allow(isInteger, "value", "li")
	p.allow(nil, "", "dl dt dd")
	p.allow(isNumberOrPercent, "height width", "table col colgroup td th img")
	p.allow(isText, "summary", "table")
	p.allow(nil, "", "caption")
	p.allow(isCellAlign, "align", "col colgroup thead tr td th tbody tfoot")
	p.allow(isInteger, "span", "col colgroup")
	p.allow(isCellVerticalAlign, "valign", "col colgroup thead tr td th tbody tfoot")
	p.allow(isText, "abbr", "td th")
	p.allow(isInteger, "colspan rowspan", "td th")
	p.allow(spaceSeparatedTokensPattern.Match, "headers", "td th")
	p.allow(scopePattern.Match, "scope", "td th")
	p.allow(nil, "nowrap", "td th")
	p.allow(numberPattern.Match, "value min max low high optimum", "meter")
	p.allow(numberPattern.Match, "value max", "progress")
	p.allow(isImageAlign, "align", "img")
	p.allow(nil, "src", "img")

	if markdown {
		p.allow(codeClassPattern.Match, "class", "code")
		p.allow(isCheckbox, "type", "input")
		p.allow(nil, "checked disabled", "input")
		p.allow(isAlertClass, "class", "div")
		p.allow(isAlertTitleClass, "class", "p")
		// Alignment of blocks in READMEs, e.g., <div align="center">, as on
		// GitHub.
		p.allow(isBlockAlign, "align", "div p h1 h2 h3 h4 h5 h6")
		// Image sizes in pixels, e.g., width="201px", which browsers read
		// as width="201".
		p.allow(isPixels, "width height", "img")
		p.allow(nil, "", "kbd")
		p.allow(isInteger, "start", "ol")
		p.allow(isTaskListClass, "class", "ul ol")
		p.allow(isTaskListItemClass, "class", "li")
		p.allow(isSourceLines, "data-source-lines", "li")
		p.allow(isFootnotesClass, "class", "div")
		p.allow(isFootnoteLinkClass, "class", "a")
		p.allow(isFootnoteRole, "role", "div a")

		p.dataURIs = true
		for _, scheme := range customSchemes {
			// Data URIs are always checked, even if listed.
			if scheme = strings.ToLower(scheme); scheme != "data" {
				p.schemes[scheme] = true
			}
		}
	}

	globals := map[string]func([]byte) bool{"dir": isDirection, "lang": hasLanguage, "id": hasID, "title": isText}
	for _, ep := range p.elements {
		for name, check := range globals {
			p.allowOn(ep, name, check)
		}
	}
	for _, name := range bareElements {
		if ep := p.elements[name]; ep != nil {
			ep.bare = true
		}
	}
	for el, name := range map[string]string{"a": "href", "area": "href", "blockquote": "cite", "q": "cite", "del": "cite", "ins": "cite", "img": "src"} {
		p.elements[el].attrs[name].url = true
	}
	p.elements["a"].link = true
	p.elements["area"].link = true
	return p
}

// allow allows the space-separated elements, with the space-separated
// attributes if their values pass check.
func (p *Policy) allow(check func([]byte) bool, attrs, elements string) {
	for _, el := range strings.Fields(elements) {
		ep := p.elements[el]
		if ep == nil {
			ep = &elementPolicy{name: el, attrs: map[string]*attrPolicy{}}
			p.elements[el] = ep
		}
		for _, name := range strings.Fields(attrs) {
			p.allowOn(ep, name, check)
		}
	}
}

// allowOn allows the attribute on the element if its value passes check or
// any check that allows it already.
func (p *Policy) allowOn(ep *elementPolicy, name string, check func([]byte) bool) {
	ap := ep.attrs[name]
	if ap == nil {
		ep.attrs[name] = &attrPolicy{name: name, check: check}
		return
	}
	if ap.check == nil || check == nil {
		ap.check = nil
		return
	}
	prev := ap.check
	ap.check = func(v []byte) bool { return prev(v) || check(v) }
}

// Sanitizer returns the parts of p that Render needs to sanitize documents
// without raw HTML while it renders them.
func (p *Policy) Sanitizer() *Sanitizer {
	return &Sanitizer{AllowURLScheme: p.allowURLScheme, Text: paragraphPattern.Regexp(), CodeClass: codeClassPattern.Regexp()}
}

// allowURLScheme reports whether p allows u, which has a scheme. Data URIs
// are checked by validating u, the others by looking their schemes up.
func (p *Policy) allowURLScheme(u *url.URL) bool {
	if u.Scheme == "data" {
		_, ok := p.validURL(u.String())
		return ok
	}
	return p.schemes[u.Scheme]
}

// validURL validates and normalizes a URL. URLs must parse and use an
// allowed scheme, or be relative. They cannot contain whitespace, except
// data URIs.
func (p *Policy) validURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if strings.ContainsAny(raw, " \t\n") {
		if !strings.HasPrefix(raw, "data:") {
			return "", false
		}
		if m := dataURIBase64Prefix.FindString(raw); m != "" {
			raw = m + strings.ReplaceAll(strings.ReplaceAll(raw[len(m):], "\r", ""), "\n", "")
		}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	if u.Scheme != "" {
		if p.schemes[u.Scheme] || u.Scheme == "data" && p.dataURIs && isSafeDataURI(u) {
			return u.String(), true
		}
		return "", false
	}
	v := u.String()
	return v, v != ""
}

// sanitizeState is the scratch space of a sanitize call.
type sanitizeState struct {
	z     htmlTokenizer
	name  []byte
	key   []byte
	attrs []cleanAttr
	// vals holds the values of attrs.
	vals []byte
	// stack are the names of the elements whose start tags were removed
	// for lack of attributes. Their end tags are removed too, matching the
	// innermost first.
	stack []string
}

type cleanAttr struct {
	name       string
	start, end int
}

var sanitizeStates = sync.Pool{New: func() any { return new(sanitizeState) }}

// AppendSanitized appends the sanitized HTML of src to dst and returns the
// extended buffer.
func (p *Policy) AppendSanitized(dst, src []byte) []byte {
	st := sanitizeStates.Get().(*sanitizeState)
	defer func() {
		// Pooled states must not keep inputs alive.
		st.z.reset(nil)
		sanitizeStates.Put(st)
	}()
	st.stack = st.stack[:0]
	z := &st.z
	z.reset(src)

	var (
		// skipping is whether the content of an element in skipContent is
		// being removed, and skipDepth how many such elements are open.
		// The depth counts end tags without start tags too, so that
		// "</iframe><iframe>" keeps skipping until the end.
		skipping  bool
		skipDepth int
		// rawText is "script" or "style" if the last start tag is one of
		// them and has not been closed, whose text is removed.
		rawText string
	)
	for {
		tt := z.next()
		switch tt {
		case html.ErrorToken:
			return dst

		case html.TextToken:
			if skipping || rawText != "" {
				continue
			}
			dst = st.appendText(dst)

		case html.StartTagToken, html.SelfClosingTagToken:
			name := st.tagName()
			start := tt == html.StartTagToken
			if string(name) == "script" || string(name) == "style" {
				if start {
					rawText = string(name)
				}
				continue
			}
			if start {
				rawText = ""
			}
			ep := p.elements[string(name)]
			if ep == nil {
				if start && skipContent[string(name)] {
					skipping = true
					skipDepth++
				}
				continue
			}
			attrs := st.sanitizeAttrs(p, ep)
			if len(attrs) == 0 && !ep.bare {
				if start {
					st.stack = append(st.stack, ep.name)
				}
				continue
			}
			if skipping {
				continue
			}
			dst = append(dst, '<')
			dst = append(dst, ep.name...)
			for _, a := range attrs {
				dst = append(dst, ' ')
				dst = append(dst, a.name...)
				dst = append(dst, `="`...)
				dst = appendEscaped(dst, st.vals[a.start:a.end])
				dst = append(dst, '"')
			}
			if start {
				dst = append(dst, '>')
			} else {
				dst = append(dst, "/>"...)
			}

		case html.EndTagToken:
			name := st.tagName()
			if rawText != "" && string(name) == rawText {
				rawText = ""
			}
			if string(name) == "script" || string(name) == "style" {
				continue
			}
			if n := len(st.stack); n > 0 && st.stack[n-1] == string(name) {
				st.stack = st.stack[:n-1]
				continue
			}
			ep := p.elements[string(name)]
			if ep == nil {
				if skipContent[string(name)] {
					skipDepth--
					if skipDepth == 0 {
						skipping = false
					}
				}
				continue
			}
			if !skipping {
				dst = append(dst, "</"...)
				dst = append(dst, ep.name...)
				dst = append(dst, '>')
			}
		}
	}
}

// tagName returns the name of the current tag token, lower-cased in ASCII
// with NUL bytes replaced by U+FFFD, as html.Tokenizer returns it.
func (st *sanitizeState) tagName() []byte {
	z := &st.z
	b := z.buf[z.data.start:z.data.end]
	for _, c := range b {
		if 'A' <= c && c <= 'Z' || c == 0 {
			st.name = st.name[:0]
			for _, c := range b {
				switch {
				case 'A' <= c && c <= 'Z':
					st.name = append(st.name, c+'a'-'A')
				case c == 0:
					st.name = append(st.name, "\ufffd"...)
				default:
					st.name = append(st.name, c)
				}
			}
			return st.name
		}
	}
	return b
}

// appendText appends the current text token, escaped.
func (st *sanitizeState) appendText(dst []byte) []byte {
	z := &st.z
	return appendTextEscaped(dst, z.buf[z.data.start:z.data.end], z.convertNUL, !z.textIsRaw)
}

// sanitizeAttrs returns the allowed attributes of the current start tag for
// the element, with the values in st.vals.
func (st *sanitizeState) sanitizeAttrs(p *Policy, ep *elementPolicy) []cleanAttr {
	z := &st.z
	st.attrs = st.attrs[:0]
	st.vals = st.vals[:0]
	href := false
	for _, a := range z.attr {
		st.key = append(st.key[:0], z.buf[a[0].start:a[0].end]...)
		for i, c := range st.key {
			if 'A' <= c && c <= 'Z' {
				st.key[i] = c + 'a' - 'A'
			}
		}
		ap := ep.attrs[string(st.key)]
		if ap == nil {
			continue
		}
		start := len(st.vals)
		st.vals = appendDecoded(st.vals, z.buf[a[1].start:a[1].end], true, true, true)
		if ap.check != nil && !ap.check(st.vals[start:]) {
			st.vals = st.vals[:start]
			continue
		}
		if ap.url {
			u, ok := p.validURL(string(st.vals[start:]))
			st.vals = st.vals[:start]
			if !ok {
				continue
			}
			st.vals = append(st.vals, u...)
			href = href || ap.name == "href"
		}
		st.attrs = append(st.attrs, cleanAttr{name: ap.name, start: start, end: len(st.vals)})
	}
	if !ep.link || !href {
		return st.attrs
	}
	for i, a := range st.attrs {
		if a.name != "rel" {
			continue
		}
		if !bytes.Contains(st.vals[a.start:a.end], []byte("nofollow")) {
			start := len(st.vals)
			st.vals = append(st.vals, st.vals[a.start:a.end]...)
			st.vals = append(st.vals, " nofollow"...)
			st.attrs[i] = cleanAttr{name: "rel", start: start, end: len(st.vals)}
		}
		return st.attrs
	}
	start := len(st.vals)
	st.vals = append(st.vals, "nofollow"...)
	return append(st.attrs, cleanAttr{name: "rel", start: start, end: len(st.vals)})
}

// Patterns of attribute values that are rare enough not to need faster
// checks.
var (
	codeClassPattern            = lazyregexp.New(`^language-[\w.+#-]+$`)
	dataURIBase64Prefix         = lazyregexp.New(`^data:[^,]*;base64,`)
	mapNamePattern              = lazyregexp.New(`^([\p{L}\p{N}_-]+)$`)
	coordsPattern               = lazyregexp.New(`^([0-9]+,)+[0-9]+$`)
	spaceSeparatedTokensPattern = lazyregexp.New(`^([\s\p{L}\p{N}_-]+)$`)
	shapePattern                = lazyregexp.New(`(?i)^(default|circle|rect|poly)$`)
	usemapPattern               = lazyregexp.New(`(?i)^#[\p{L}\p{N}_-]+$`)
	iso8601Pattern              = lazyregexp.New(`^[0-9]{4}(-[0-9]{2}(-[0-9]{2}([ T][0-9]{2}(:[0-9]{2}){1,2}(.[0-9]{1,6})?Z?([\+-][0-9]{2}:[0-9]{2})?)?)?)?$`)
	listTypePattern             = lazyregexp.New(`(?i)^(circle|disc|square|a|A|i|I|1)$`)
	scopePattern                = lazyregexp.New(`(?i)(?:row|col)(?:group)?`)
	numberPattern               = lazyregexp.New(`^[-+]?[0-9]*\.?[0-9]+([eE][-+]?[0-9]+)?$`)
	// paragraphPattern is the pattern of isText, for Dariomark.
	paragraphPattern = lazyregexp.New(`^[\p{L}\p{N}\s\-_',\[\]!\./\\\(\)]*$`)
)

// textASCII are the ASCII characters that isText allows.
var textASCII = func() (t [utf8.RuneSelf]bool) {
	for _, c := range "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789\t\n\f\r -_',[]!./\\()" {
		t[c] = true
	}
	return t
}()

// isText reports whether v is plain text, i.e., letters, numbers, white
// space and some punctuation. It matches paragraphPattern.
func isText(v []byte) bool {
	for i := 0; i < len(v); {
		if c := v[i]; c < utf8.RuneSelf {
			if !textASCII[c] {
				return false
			}
			i++
			continue
		}
		r, n := utf8.DecodeRune(v[i:])
		if !unicode.IsLetter(r) && !unicode.IsNumber(r) {
			return false
		}
		i += n
	}
	return true
}

// hasID reports whether v has a character of an id, as the unanchored
// pattern `[a-zA-Z0-9\:\-_\.]+` does.
func hasID(v []byte) bool {
	for _, c := range v {
		if isASCIIAlnum(c) || c == ':' || c == '-' || c == '_' || c == '.' {
			return true
		}
	}
	return false
}

// hasLanguage reports whether v has two ASCII letters in a row, as the
// unanchored pattern `[a-zA-Z]{2,20}` does.
func hasLanguage(v []byte) bool {
	letters := 0
	for _, c := range v {
		if 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' {
			if letters++; letters == 2 {
				return true
			}
		} else {
			letters = 0
		}
	}
	return false
}

func isDigits(v []byte) bool {
	for _, c := range v {
		if !isASCIIDigit(c) {
			return false
		}
	}
	return len(v) > 0
}

func isInteger(v []byte) bool {
	return isDigits(v)
}

func isNumberOrPercent(v []byte) bool {
	return isDigits(bytes.TrimSuffix(v, []byte("%")))
}

func isPixels(v []byte) bool {
	n := len(v)
	return n > 2 && (v[n-2]|0x20) == 'p' && (v[n-1]|0x20) == 'x' && isDigits(v[:n-2])
}

func isSourceLines(v []byte) bool {
	a, b, ok := bytes.Cut(v, []byte("-"))
	return ok && isDigits(a) && isDigits(b)
}

func isCheckbox(v []byte) bool {
	return string(v) == "checkbox"
}

func isOpen(v []byte) bool {
	return len(v) == 0 || equalFoldWord(v, "open")
}

func isDirection(v []byte) bool {
	return equalFoldWord(v, "rtl") || equalFoldWord(v, "ltr")
}

func isCellAlign(v []byte) bool {
	return equalFoldAny(v, "center", "justify", "left", "right", "char")
}

func isCellVerticalAlign(v []byte) bool {
	return equalFoldAny(v, "baseline", "bottom", "middle", "top")
}

func isImageAlign(v []byte) bool {
	return equalFoldAny(v, "left", "right", "top", "texttop", "middle", "absmiddle", "baseline", "bottom", "absbottom")
}

func isBlockAlign(v []byte) bool {
	return equalFoldAny(v, "left", "center", "right", "justify")
}

func isAlertClass(v []byte) bool {
	switch string(v) {
	case "markdown-alert markdown-alert-note", "markdown-alert markdown-alert-tip", "markdown-alert markdown-alert-important",
		"markdown-alert markdown-alert-warning", "markdown-alert markdown-alert-caution":
		return true
	}
	return false
}

func isAlertTitleClass(v []byte) bool {
	return string(v) == "markdown-alert-title"
}

func isTaskListClass(v []byte) bool {
	return string(v) == "contains-task-list"
}

func isTaskListItemClass(v []byte) bool {
	return string(v) == "task-list-item"
}

func isFootnotesClass(v []byte) bool {
	return string(v) == "footnotes"
}

func isFootnoteLinkClass(v []byte) bool {
	return string(v) == "footnote-ref" || string(v) == "footnote-backref"
}

func isFootnoteRole(v []byte) bool {
	switch string(v) {
	case "doc-endnotes", "doc-noteref", "doc-backlink":
		return true
	}
	return false
}

func equalFoldAny(v []byte, words ...string) bool {
	for _, w := range words {
		if equalFoldWord(v, w) {
			return true
		}
	}
	return false
}

// equalFoldWord reports whether v is the lower-case ASCII word, ignoring case
// as the (?i) flag of regexps does, which also folds "ſ" (U+017F) with "s"
// and the Kelvin sign (U+212A) with "k".
func equalFoldWord(v []byte, word string) bool {
	i := 0
	for j := 0; j < len(word); j++ {
		if i >= len(v) {
			return false
		}
		c := word[j]
		if b := v[i]; b < utf8.RuneSelf {
			if b != c && !('a' <= c && c <= 'z' && b == c-('a'-'A')) {
				return false
			}
			i++
			continue
		}
		r, n := utf8.DecodeRune(v[i:])
		if !(c == 's' && r == '\u017f' || c == 'k' && r == '\u212a') {
			return false
		}
		i += n
	}
	return i == len(v)
}
