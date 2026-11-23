package markup

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"path/filepath"
	"strings"

	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/lazyregexp"
	"gogs.io/gogs/internal/markup/dariomark"
)

// IsMarkdownFile reports whether name has one of the Markdown file extensions.
func IsMarkdownFile(extensions []string, name string) bool {
	extension := strings.ToLower(filepath.Ext(name))
	for _, ext := range extensions {
		if strings.ToLower(ext) == extension {
			return true
		}
	}
	return false
}

var validLinksPattern = lazyregexp.New(`^[a-z][\w-]+://|^mailto:|^//`)

// isLink reports whether link is absolute, i.e., has a scheme or is
// protocol-relative.
func isLink(link []byte) bool {
	return validLinksPattern.Match(link)
}

// linkifyURL returns the length of the URL at the start of b, or 0 if there is
// none. It does not require a top-level domain, so links to hosts like
// "localhost:3000" are also detected. It matches what the regular expression
// below matches, which is too slow to run on every URL:
//
//	^(?:http|https|ftp)://[-a-zA-Z0-9@:%._\+~#=]{1,256}(?:[/#?][-a-zA-Z0-9@:%_+.~#$!?&/=\(\);,'">\^{}\[\]`]*)?
func linkifyURL(b []byte) int {
	var n int
	for _, scheme := range []string{"http://", "https://", "ftp://"} {
		if bytes.HasPrefix(b, []byte(scheme)) {
			n = len(scheme)
			break
		}
	}
	if n == 0 {
		return 0
	}
	host := n
	for n < len(b) && n-host < 256 && isURLHostChar(b[n]) {
		n++
	}
	if n == host {
		return 0
	}
	if n < len(b) && (b[n] == '/' || b[n] == '#' || b[n] == '?') {
		n++
		for n < len(b) && isURLPathChar(b[n]) {
			n++
		}
	}
	return n
}

func isURLHostChar(c byte) bool {
	return isASCIIAlnum(c) || strings.IndexByte("-@:%._+~#=", c) >= 0
}

func isURLPathChar(c byte) bool {
	return isASCIIAlnum(c) || strings.IndexByte("-@:%_+.~#$!?&/=();,'\">^{}[]`", c) >= 0
}

// shortenAutoLink renders links to commits and issues on this instance in a
// short form, e.g., "d8a994ef24" and "#12".
func shortenAutoLink(o Options, link []byte, urlPrefix string) ([]byte, bool) {
	if !bytes.HasPrefix(link, []byte(o.ExternalURL)) {
		return nil, false
	}

	if m := findAutoLinkPath(link, CommitPattern, "commit/", isASCIIAlnum); m != nil {
		m = bytes.TrimSpace(m)
		i := bytes.Index(m, []byte("commit/"))
		j := bytes.Index(m, []byte("#"))
		if j == -1 {
			j = len(m)
		}
		if j < i+7 {
			// The hash precedes the commit path, e.g., "/#commit/1".
			return nil, false
		}
		html := append([]byte(`<code><a href="`), dariomark.AppendEscaped(nil, m)...)
		html = append(html, `">`...)
		html = append(html, git.ShortID(string(m[i+7:j]))...)
		return append(html, "</a></code>"...), true
	}

	if m := findAutoLinkPath(link, IssueFullPattern, "issues/", isDigit); m != nil {
		m = bytes.TrimSpace(m)
		i := bytes.Index(m, []byte("issues/"))
		j := bytes.Index(m, []byte("#"))
		if j == -1 {
			j = len(m)
		}
		if j < i+7 {
			// The hash precedes the issue path, e.g., "/#issues/1".
			return nil, false
		}

		index := m[i+7 : j]
		fullRepoURL := o.ExternalURL + strings.TrimPrefix(urlPrefix, "/")
		html := append([]byte(`<a href="`), dariomark.AppendEscaped(nil, m)...)
		if bytes.HasPrefix(m, []byte(fullRepoURL)) {
			// Use a short issue reference if the URL refers to this repository.
			html = append(html, `">#`...)
		} else {
			// Use a cross-repository issue reference if the URL refers to a different repository.
			if i-1 < len(o.ExternalURL) {
				// There is no repository path, e.g., "/issues/1".
				return nil, false
			}
			html = append(html, `">`...)
			html = dariomark.AppendEscaped(html, m[len(o.ExternalURL):i-1])
			html = append(html, '#')
		}
		html = append(html, index...)
		return append(html, "</a>"...), true
	}
	return nil, false
}

// findAutoLinkPath returns what pattern finds in link, where pattern is
// CommitPattern or IssueFullPattern and path is the literal it requires before
// the ID. Links without whitespace, which are all links in practice, are
// matched without running the slow backtracking pattern.
func findAutoLinkPath(link []byte, pattern *lazyregexp.Regexp, path string, isID func(byte) bool) []byte {
	if bytes.ContainsAny(link, " \t\n\f\r") {
		return pattern.Find(link)
	}
	if !bytes.HasPrefix(link, []byte("http")) {
		return nil
	}
	// The greedy ".*" after "http" makes the last path followed by an ID win.
	end := len(link)
	for {
		i := bytes.LastIndex(link[4:end], []byte(path))
		if i < 0 {
			return nil
		}
		j := 4 + i + len(path)
		if j < len(link) && isID(link[j]) {
			for j < len(link) && isID(link[j]) {
				j++
			}
			if j < len(link) && link[j] == '#' {
				for j < len(link) && link[j] == '#' {
					j++
				}
				for j < len(link) && (isASCIIAlnum(link[j]) || link[j] == '-') {
					j++
				}
			}
			return link[:j]
		}
		end = j - 1
	}
}

func isASCIIAlnum(c byte) bool {
	return isDigit(c) || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}

// markdownOptions returns the rendering options for Markdown without
// post-processing. hardWraps renders every line break as written.
func markdownOptions(o Options, urlPrefix string, hardWraps bool) *dariomark.Options {
	opts := &dariomark.Options{
		Tables:                    true,
		Strikethrough:             true,
		TaskLists:                 true,
		Linkify:                   true,
		LinkifyURL:                linkifyURL,
		LinkifyAfterPunct:         true,
		Footnotes:                 true,
		Alerts:                    true,
		HeadingIDs:                true,
		HeadingIDPrefix:           "user-content-",
		HardWraps:                 hardWraps,
		RelativeLinks:             true,
		LinkBase:                  urlPrefix,
		IsLink:                    isLink,
		TextAutoLinksInRawAnchors: true,
		ShortenAutoLink: func(link []byte) ([]byte, bool) {
			return shortenAutoLink(o, link, urlPrefix)
		},
	}
	if o.Smartypants.Enabled {
		opts.Typographer = dariomark.Typographer{
			Enabled:      true,
			Dashes:       o.Smartypants.Dashes,
			LatexDashes:  o.Smartypants.LatexDashes,
			AngledQuotes: o.Smartypants.AngledQuotes,
			Fractions:    o.Smartypants.Fractions,
		}
	}
	return opts
}

// footnoteIDPrefix returns the prefix of the footnote ids of body. The
// "user-content-" part keeps the ids from clobbering ids of the page, and the
// hash keeps footnotes of documents shown on the same page, e.g., comments of
// an issue, from linking to each other.
func footnoteIDPrefix(body []byte) string {
	h := fnv.New32a()
	_, _ = h.Write(body)
	return fmt.Sprintf("user-content-%08x-", h.Sum32())
}

// RawMarkdown renders content in Markdown syntax to HTML without handling special links.
// The output is not sanitized. Like MarkdownFile, it keeps line breaks only
// with Options.FileHardLineBreak, and shows a document larger than
// Options.MaxDisplayFileSize as escaped plain text.
func RawMarkdown(o Options, body []byte, urlPrefix string) []byte {
	body = bytes.TrimPrefix(body, utf8BOM)
	return guardRender(o, body, func() []byte {
		opts := markdownOptions(o, urlPrefix, o.FileHardLineBreak)
		opts.FootnoteIDPrefix = footnoteIDPrefix(body)
		return dariomark.HTML(body, opts)
	})
}

// renderMarkdown renders content in Markdown syntax to HTML with special
// links, see dariomark.Render. file renders YAML front matter as a table, for
// Markdown files. commits, when not nil, are the commits that the hex strings
// in body name, see ResolveCommits.
func renderMarkdown(o Options, body []byte, urlPrefix string, metas, commits map[string]string, hardWraps, file bool) dariomark.Result {
	opts := markdownRenderOptions(o, body, urlPrefix, hardWraps, file)
	if commits == nil {
		commits = resolveLinkableCommits(metas, func(specialText func(text []byte)) {
			renderLinkableText(body, opts, specialText)
		})
	}
	opts.SpecialLinks = func(text []byte) []dariomark.SpecialLink {
		return findSpecialLinks(o, text, urlPrefix, metas, commits)
	}
	return dariomark.Render(body, opts)
}

// markdownRenderOptions returns the options of renderMarkdown, without special
// links.
func markdownRenderOptions(o Options, body []byte, urlPrefix string, hardWraps, file bool) *dariomark.Options {
	opts := markdownOptions(o, urlPrefix, hardWraps)
	opts.FootnoteIDPrefix = footnoteIDPrefix(body)
	opts.PostProcess = true
	opts.Emoji = emoji
	if file {
		opts.FrontMatter = true
	}
	if sanitizer.ready.Load() {
		opts.Sanitizer = sanitizer.markdown
	}
	return opts
}

// renderLinkableText renders body with opts, passing the text where special
// links render to specialText, when body has commit ID candidates.
func renderLinkableText(body []byte, opts *dariomark.Options, specialText func(text []byte)) {
	if !Sha1CurrentPattern.Match(body) {
		return
	}
	collect := *opts
	collect.SpecialLinks = func(text []byte) []dariomark.SpecialLink {
		specialText(text)
		return nil
	}
	dariomark.Render(body, &collect)
}

// ResolveCommits returns the commits that the hex strings in the documents,
// e.g., the comments of an issue, name in the repository of metas, with one
// Git command for all of them. Rendering each document with
// MarkdownWithCommits and the result then runs no Git command.
func ResolveCommits(o Options, metas map[string]string, docs ...string) map[string]string {
	commits := resolveLinkableCommits(metas, func(specialText func(text []byte)) {
		for _, doc := range docs {
			body := bytes.TrimPrefix([]byte(doc), utf8BOM)
			guardRender(o, body, func() []byte {
				renderLinkableText(body, markdownRenderOptions(o, body, "", true, false), specialText)
				return nil
			})
		}
	})
	if commits == nil {
		// Not nil, so that rendering does not resolve the commits again.
		return map[string]string{}
	}
	return commits
}

// Kinds of special links, see specialLinkKinds.
const (
	linkMention = 1 << iota
	linkIssue
	linkCrossReference
	linkCommit

	allLinkKinds = linkMention | linkIssue | linkCrossReference | linkCommit
)

// specialLinkKinds returns the kinds of special links that text may have. For
// each kind, it checks a character sequence that its pattern requires, which
// is much faster than running the pattern. Issue references of a tracker
// style it does not know are always looked for.
func specialLinkKinds(text []byte, metas map[string]string) int {
	kinds := 0
	style := metas["style"]
	switch style {
	case "", IssueNameStyleNumeric, IssueNameStyleAlphanumeric:
	default:
		kinds |= linkIssue
	}
	alphanumeric := style == IssueNameStyleAlphanumeric
	hexRun, slash := 0, false
	for i, c := range text {
		next := byte(0)
		if i+1 < len(text) {
			next = text[i+1]
		}
		switch c {
		case '@':
			if isMentionChar(next) {
				kinds |= linkMention
			}
		case '#':
			if !isDigit(next) {
				break
			}
			if !alphanumeric && (i == 0 || strings.IndexByte(" ([", text[i-1]) >= 0) {
				kinds |= linkIssue
			}
			if slash && i > 0 && isMentionChar(text[i-1]) {
				kinds |= linkCrossReference
			}
		case '-':
			if alphanumeric && i > 0 && 'A' <= text[i-1] && text[i-1] <= 'Z' && '1' <= next && next <= '9' {
				kinds |= linkIssue
			}
		case '/':
			slash = true
		}
		if isDigit(c) || 'a' <= c && c <= 'f' {
			hexRun++
			if hexRun >= 7 {
				kinds |= linkCommit
			}
		} else {
			hexRun = 0
		}
		if kinds == allLinkKinds {
			break
		}
	}
	return kinds
}

func isDigit(c byte) bool {
	return '0' <= c && c <= '9'
}

// isMentionChar reports whether c can be part of a user name in a mention.
func isMentionChar(c byte) bool {
	return isASCIIAlnum(c) || c == '-' || c == '_' || c == '.'
}

// Markdown takes a string or []byte of Markdown that is not a file, e.g., an
// issue, a comment or a release note, and renders it to HTML with special
// links. It renders every line break as written.
func Markdown(o Options, input any, urlPrefix string, metas map[string]string) []byte {
	return renderInput(o, TypeMarkdown, input, urlPrefix, metas, nil, true, false)
}

// MarkdownWithCommits is Markdown with the commits that ResolveCommits
// returned for input, among other documents.
func MarkdownWithCommits(o Options, input any, urlPrefix string, metas, commits map[string]string) []byte {
	return renderInput(o, TypeMarkdown, input, urlPrefix, metas, commits, true, false)
}

// MarkdownFile is Markdown for Markdown files, e.g., READMEs and wiki pages.
// It joins the lines of a paragraph, unless Options.FileHardLineBreak is set,
// and renders YAML front matter as a table above the document.
func MarkdownFile(o Options, input any, urlPrefix string, metas map[string]string) []byte {
	return Render(o, TypeMarkdown, input, urlPrefix, metas)
}
