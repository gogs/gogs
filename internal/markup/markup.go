package markup

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/sync/semaphore"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/lazyregexp"
	"gogs.io/gogs/internal/markup/dariomark"
)

// IsReadmeFile reports whether name looks like a README file based on its extension.
func IsReadmeFile(name string) bool {
	return strings.HasPrefix(strings.ToLower(name), "readme")
}

// IsIPythonNotebook reports whether name looks like an IPython notebook based on its extension.
func IsIPythonNotebook(name string) bool {
	return strings.HasSuffix(name, ".ipynb")
}

const (
	IssueNameStyleNumeric      = "numeric"
	IssueNameStyleAlphanumeric = "alphanumeric"
)

var (
	// MentionPattern matches string that mentions someone, e.g., @Unknwon
	MentionPattern = lazyregexp.New(`(\s|^|\W)@[0-9a-zA-Z-_\.]+`)
	// urlPattern matches URLs in plain text, in which "@" does not start a
	// mention, e.g., https://medium.com/@alice.
	urlPattern = lazyregexp.New(`(?i)\b[a-z][a-z0-9+.-]*://\S+`)

	// CommitPattern matches link to certain commit with or without trailing hash,
	// e.g., https://try.gogs.io/gogs/gogs/commit/d8a994ef243349f321568f9e36d5c3f444b99cae#diff-2
	CommitPattern = lazyregexp.New(`(\s|^)https?.*commit/[0-9a-zA-Z]+(#+[0-9a-zA-Z-]*)?`)

	// IssueFullPattern matches link to an issue with or without trailing hash,
	// e.g., https://try.gogs.io/gogs/gogs/issues/4#issue-685
	IssueFullPattern = lazyregexp.New(`(\s|^)https?.*issues/[0-9]+(#+[0-9a-zA-Z-]*)?`)
	// IssueNumericPattern matches string that references a numeric issue, e.g., #1287
	IssueNumericPattern = lazyregexp.New(`( |^|\(|\[)#[0-9]+\b`)
	// IssueAlphanumericPattern matches string that references an alphanumeric issue, e.g., ABC-1234
	IssueAlphanumericPattern = lazyregexp.New(`( |^|\(|\[)[A-Z]{1,10}-[1-9][0-9]*\b`)
	// CrossReferenceIssueNumericPattern matches string that references a numeric issue in a different repository,
	// e.g., gogs/gogs#12345
	CrossReferenceIssueNumericPattern = lazyregexp.New(`( |^)[0-9a-zA-Z-_\.]+/[0-9a-zA-Z-_\.]+#[0-9]+\b`)

	// Sha1CurrentPattern matches string that represents a commit SHA, e.g., d8a994ef243349f321568f9e36d5c3f444b99cae
	// It only finds candidates, which are linked when they name a commit of the
	// repository, see resolveCommits. Pure numbers are never linked.
	Sha1CurrentPattern = lazyregexp.New(`\b(?:[0-9a-f]{7,40}|[0-9a-f]{64})\b`)
)

// FindAllMentions returns the user names, without "@", of the mentions that
// render as links in content, which is Markdown. Mentions in code and links
// are not included.
func FindAllMentions(content string) []string {
	body := bytes.TrimPrefix([]byte(content), utf8BOM)
	var names []string
	guardRender(body, func() []byte {
		opts := markdownOptions("", true)
		opts.PostProcess = true
		opts.SpecialLinks = func(text []byte) []dariomark.SpecialLink {
			if bytes.IndexByte(text, '@') >= 0 {
				for _, m := range findMentions(text) {
					names = append(names, string(m.Text[1:]))
				}
			}
			return nil
		}
		dariomark.Render(body, opts)
		return nil
	})
	return names
}

// findMentions returns the mentions in text.
func findMentions(text []byte) []linkSpan {
	var spans []linkSpan
	for _, loc := range MentionPattern.Regexp().FindAllSubmatchIndex(text, -1) {
		// The pattern requires a character before the "@", which is not part
		// of the mention.
		start := loc[3]
		m := text[start:loc[1]]
		spans = append(spans, linkSpan{Start: start, End: loc[1], Href: fmt.Appendf(nil, "%s/%s", html.EscapeString(conf.Server.Subpath), m[1:]), Text: m})
	}
	return spans
}

// cutoutVerbosePrefix cuts out the URL prefix including the subpath to return
// a clean unified string of request URL path.
func cutoutVerbosePrefix(prefix string) string {
	if prefix == "" || prefix[0] != '/' {
		return prefix
	}
	count := 0
	for i := 0; i < len(prefix); i++ {
		if prefix[i] == '/' {
			count++
		}
		if count >= 3+conf.Server.SubpathDepth {
			return prefix[:i]
		}
	}
	return prefix
}

// expand substitutes "{key}" placeholders in template with values from match,
// in one pass from left to right like a strings.Replacer, given that no key
// contains "}". It runs for every issue reference, so it does not build a
// strings.Replacer, and it takes linear time for any template.
func expand(template string, match map[string]string) string {
	maxKey := 0
	for k := range match {
		maxKey = max(maxKey, len(k))
	}
	var b strings.Builder
	written := 0
	// end is the first "}" at or after the current "{", or -1.
	end := -1
	for i := 0; i < len(template); i++ {
		if template[i] != '{' {
			continue
		}
		if end < i {
			end = strings.IndexByte(template[i:], '}')
			if end < 0 {
				break
			}
			end += i
		}
		if end-i-1 > maxKey {
			continue
		}
		v, ok := match[template[i+1:end]]
		if !ok {
			continue
		}
		b.WriteString(template[written:i])
		b.WriteString(v)
		written = end + 1
		i = end
	}
	b.WriteString(template[written:])
	return b.String()
}

// linkSpan is a reference found in text and the link that replaces it.
type linkSpan = dariomark.SpecialLink

// findIssueIndexes returns the spans of skip merged with the issue references
// in text that do not overlap them.
func findIssueIndexes(text []byte, urlPrefix string, metas map[string]string, skip []linkSpan) []linkSpan {
	urlPrefix = cutoutVerbosePrefix(urlPrefix)

	pattern := IssueNumericPattern
	alphanumeric := metas["style"] == IssueNameStyleAlphanumeric
	if alphanumeric {
		pattern = IssueAlphanumericPattern
	}

	// External issue tracker formats are expanded with a copy of metas, so
	// that callers sharing metas between renders are not affected.
	var match map[string]string
	if metas["format"] != "" {
		match = make(map[string]string, len(metas)+1)
		for k, v := range metas {
			match[k] = v
		}
	}

	var spans []linkSpan
	for _, loc := range pattern.FindAllIndex(text, -1) {
		start := loc[0]
		if c := text[start]; c == ' ' || c == '(' || c == '[' {
			start++
		}
		m := text[start:loc[1]]
		var href []byte
		if match == nil {
			href = fmt.Appendf(nil, "%s/issues/%s", html.EscapeString(urlPrefix), m[1:])
		} else {
			if alphanumeric {
				match["index"] = string(m)
			} else {
				match["index"] = string(m[1:])
			}
			href = []byte(html.EscapeString(expand(metas["format"], match)))
		}
		spans = append(spans, linkSpan{Start: start, End: loc[1], Href: href, Text: m})
	}
	return mergeSpans(skip, spans)
}

// mergeSpans returns the spans of both sorted lists, dropping the spans of
// more that overlap a span of spans.
func mergeSpans(spans, more []linkSpan) []linkSpan {
	if len(more) == 0 {
		return spans
	}
	merged := make([]linkSpan, 0, len(spans)+len(more))
	i := 0
	for _, s := range more {
		for i < len(spans) && spans[i].End <= s.Start {
			merged = append(merged, spans[i])
			i++
		}
		if i < len(spans) && spans[i].Start < s.End {
			continue
		}
		merged = append(merged, s)
	}
	return append(merged, spans[i:]...)
}

// replaceSpans returns text with each span replaced by its link.
func replaceSpans(text []byte, spans []linkSpan) []byte {
	if len(spans) == 0 {
		return text
	}
	return dariomark.AppendSpecialLinks(make([]byte, 0, len(text)+len(spans)*48), text, spans)
}

// RenderCommitMessage escapes msg and links the mentions and issue references
// in it.
func RenderCommitMessage(msg, urlPrefix string, metas map[string]string) string {
	text := []byte(html.EscapeString(msg))
	// Unlike Markdown, plain text does not link URLs first, which would keep
	// mentions out of them.
	urls := urlPattern.FindAllIndex(text, -1)
	mentions := slices.DeleteFunc(findMentions(text), func(m linkSpan) bool {
		return slices.ContainsFunc(urls, func(u []int) bool { return u[0] <= m.Start && m.Start < u[1] })
	})
	// Repository admins set the external issue tracker format the links are
	// made of, which can be any URL, e.g., a "javascript:" URL.
	return string(SanitizeBytes(replaceSpans(text, findIssueIndexes(text, urlPrefix, metas, mentions))))
}

// RenderIssueIndexPattern renders issue indexes to corresponding links.
func RenderIssueIndexPattern(rawBytes []byte, urlPrefix string, metas map[string]string) []byte {
	return replaceSpans(rawBytes, findIssueIndexes(rawBytes, urlPrefix, metas, nil))
}

// findCrossReferences returns the spans of skip merged with the references to
// issues of other repositories in text that do not overlap them.
func findCrossReferences(text []byte, skip []linkSpan) []linkSpan {
	var spans []linkSpan
	for _, loc := range CrossReferenceIssueNumericPattern.FindAllIndex(text, -1) {
		start := loc[0]
		if c := text[start]; c == ' ' || c == '(' {
			start++
		}
		m := text[start:loc[1]]
		delim := bytes.IndexByte(m, '#')
		href := fmt.Appendf(nil, "%s%s/issues/%s", html.EscapeString(conf.Server.ExternalURL), m[:delim], m[delim+1:])
		spans = append(spans, linkSpan{Start: start, End: loc[1], Href: href, Text: m})
	}
	return mergeSpans(skip, spans)
}

// resolveCommits returns the full IDs of the commits that the commit ID
// candidates in text name in the repository in metas["repoPath"], keyed by the
// candidate. It returns nil without a repository, or when the commits cannot be
// resolved, so that the candidates stay text.
func resolveCommits(text []byte, metas map[string]string) map[string]string {
	repoPath := metas["repoPath"]
	if repoPath == "" {
		return nil
	}
	var ids []string
	seen := make(map[string]bool)
	for _, m := range Sha1CurrentPattern.FindAll(text, -1) {
		id := string(m)
		if v, _ := strconv.Atoi(id); v > 0 || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}
	commits, err := git.RepoCommitIDs(context.Background(), repoPath, ids)
	if err != nil {
		return nil
	}
	return commits
}

// resolveLinkableCommits is resolveCommits for the text that render passes to
// specialText, which must be the text where special links render. Hex strings
// elsewhere, e.g., in code and link URLs, are never linked, so they do not run
// Git. render only runs when there is a repository to resolve them in.
func resolveLinkableCommits(metas map[string]string, render func(specialText func(text []byte))) map[string]string {
	if metas["repoPath"] == "" {
		return nil
	}
	var text []byte
	render(func(t []byte) {
		// The space keeps the candidates of adjacent text nodes apart, as
		// rendering does.
		text = append(append(text, t...), ' ')
	})
	return resolveCommits(text, metas)
}

// findSha1s returns the spans of skip merged with the commit IDs in text that
// name a commit in commits, see resolveCommits, and do not overlap them.
func findSha1s(text []byte, metas, commits map[string]string, skip []linkSpan) []linkSpan {
	var spans []linkSpan
	for _, loc := range Sha1CurrentPattern.FindAllIndex(text, -1) {
		m := text[loc[0]:loc[1]]
		fullID, ok := commits[string(m)]
		if !ok {
			continue
		}
		href := fmt.Appendf(nil, "%s/commit/%s", html.EscapeString(metas["repoLink"]), fullID)
		spans = append(spans, linkSpan{Start: loc[0], End: loc[1], Href: href, Text: m[:min(len(m), 10)], Code: true})
	}
	return mergeSpans(skip, spans)
}

// RenderSpecialLink renders mentions, indexes and commit IDs to corresponding links.
// It replaces each reference where it is found, in a single pass, and a
// reference that overlaps an earlier kind of reference is left as text.
func RenderSpecialLink(rawBytes []byte, urlPrefix string, metas map[string]string) []byte {
	return replaceSpans(rawBytes, findSpecialLinks(rawBytes, urlPrefix, metas, resolveCommits(rawBytes, metas)))
}

// findSpecialLinks returns the mentions, issue references, cross-references
// and commit IDs in text, see RenderSpecialLink. Commit IDs are linked only
// when they are in commits, see resolveCommits.
func findSpecialLinks(rawBytes []byte, urlPrefix string, metas, commits map[string]string) []linkSpan {
	kinds := specialLinkKinds(rawBytes, metas)
	if kinds == 0 {
		return nil
	}
	var spans []linkSpan
	if kinds&linkMention != 0 {
		spans = findMentions(rawBytes)
	}
	if kinds&linkIssue != 0 {
		spans = findIssueIndexes(rawBytes, urlPrefix, metas, spans)
	}
	if kinds&linkCrossReference != 0 {
		spans = findCrossReferences(rawBytes, spans)
	}
	if kinds&linkCommit != 0 {
		spans = findSha1s(rawBytes, metas, commits, spans)
	}
	return spans
}

var (
	leftAngleBracket  = []byte("</")
	rightAngleBracket = []byte(">")
)

var noEndTags = []string{"input", "br", "hr", "img"}

// wrapImgWithLink wraps standalone <img> tags with a link to the image.
func wrapImgWithLink(urlPrefix string, buf *bytes.Buffer, token html.Token) {
	var src, alt string
	for i := range token.Attr {
		switch token.Attr[i].Key {
		case "src":
			src = token.Attr[i].Val
		case "alt":
			alt = token.Attr[i].Val
		}
	}

	if src == "" {
		buf.WriteString(token.String())
		return
	}

	if strings.HasPrefix(src, "data:") {
		buf.WriteString(token.String())
		return
	}

	// Prepend repository base URL for internal links.
	needPrepend := !isLink([]byte(src))
	if needPrepend {
		urlPrefix = strings.Replace(urlPrefix, "/src/", "/raw/", 1)
		if src[0] != '/' {
			urlPrefix += "/"
		}
	}

	buf.WriteString(`<a href="`)
	if needPrepend {
		buf.WriteString(urlPrefix)
		buf.WriteString(src)
	} else {
		buf.WriteString(src)
	}
	buf.WriteString(`">`)

	if needPrepend {
		src = strings.ReplaceAll(urlPrefix+src, " ", "%20")
		buf.WriteString(`<img src="`)
		buf.WriteString(src)
		buf.WriteString(`"`)

		if len(alt) > 0 {
			buf.WriteString(` alt="`)
			buf.WriteString(alt)
			buf.WriteString(`"`)
		}

		buf.WriteString(`>`)

	} else {
		buf.WriteString(token.String())
	}

	buf.WriteString(`</a>`)
}

// postProcessHTML treats different types of HTML differently,
// and only renders special links for plain text blocks.
func postProcessHTML(rawHTML []byte, urlPrefix string, metas map[string]string) []byte {
	commits := resolveLinkableCommits(metas, func(specialText func(text []byte)) {
		if !Sha1CurrentPattern.Match(rawHTML) {
			return
		}
		processHTMLText(rawHTML, urlPrefix, func(text []byte) []byte {
			specialText(text)
			return text
		})
	})
	return processHTMLText(rawHTML, urlPrefix, func(text []byte) []byte {
		return replaceSpans(text, findSpecialLinks(text, urlPrefix, metas, commits))
	})
}

// processHTMLText is postProcessHTML, with renderText rendering the text
// where special links render.
func processHTMLText(rawHTML []byte, urlPrefix string, renderText func(text []byte) []byte) []byte {
	startTags := make([]string, 0, 5)
	buf := bytes.NewBuffer(nil)
	tokenizer := html.NewTokenizer(bytes.NewReader(rawHTML))

outerLoop:
	for html.ErrorToken != tokenizer.Next() {
		token := tokenizer.Token()
		switch token.Type {
		case html.TextToken:
			buf.Write(renderText([]byte(token.String())))

		case html.StartTagToken:
			tagName := token.Data

			if tagName == "img" {
				wrapImgWithLink(urlPrefix, buf, token)
				continue outerLoop
			}

			buf.WriteString(token.String())
			// If this is an excluded tag, we skip processing all output until a close tag is encountered.
			if strings.EqualFold("a", tagName) || strings.EqualFold("code", tagName) || strings.EqualFold("pre", tagName) {
				stackNum := 1
				for html.ErrorToken != tokenizer.Next() {
					token = tokenizer.Token()

					buf.WriteString(token.String())

					// Stack number doesn't increase for tags without end tags.
					if token.Type == html.StartTagToken && !slices.Contains(noEndTags, token.Data) {
						stackNum++
					}

					// If this is the close tag of the outermost tag, we are done.
					if token.Type == html.EndTagToken {
						stackNum--
						if stackNum <= 0 && strings.EqualFold(tagName, token.Data) {
							break
						}
					}
				}
				continue outerLoop
			}

			if !slices.Contains(noEndTags, tagName) {
				startTags = append(startTags, tagName)
			}

		case html.EndTagToken:
			if len(startTags) == 0 {
				buf.WriteString(token.String())
				break
			}

			buf.Write(leftAngleBracket)
			buf.WriteString(startTags[len(startTags)-1])
			buf.Write(rightAngleBracket)
			startTags = startTags[:len(startTags)-1]
		default:
			buf.WriteString(token.String())
		}
	}

	if io.EOF == tokenizer.Err() {
		return buf.Bytes()
	}

	// If we are not at the end of the input, then some other parsing error has occurred,
	// so return the input verbatim.
	return rawHTML
}

type Type string

const (
	TypeUnrecognized    Type = "unrecognized"
	TypeMarkdown        Type = "markdown"
	TypeOrgMode         Type = "orgmode"
	TypeIPythonNotebook Type = "ipynb"
)

// Detect returns best guess of a markup type based on file name.
func Detect(filename string) Type {
	switch {
	case IsMarkdownFile(filename):
		return TypeMarkdown
	case IsOrgModeFile(filename):
		return TypeOrgMode
	case IsIPythonNotebook(filename):
		return TypeIPythonNotebook
	default:
		return TypeUnrecognized
	}
}

const (
	// largeRenderSize is the input size from which a render takes part of
	// renderBudget. Smaller documents, such as most comments, are rendered
	// right away.
	largeRenderSize = 64 << 10
	// renderBudget is the total input size of large documents rendered at the
	// same time. Rendering takes up to about 360 times the input in memory for
	// hostile documents, so concurrent large renders stay under 3 GB. A
	// document larger than the budget waits until it can be rendered alone.
	renderBudget = 8 << 20
)

var renderSemaphore = semaphore.NewWeighted(renderBudget)

// utf8BOM is the byte order mark that some editors write at the start of
// files. It is not part of the content.
var utf8BOM = []byte("\ufeff")

// Render takes a string or []byte and renders to sanitized HTML in given type of syntax with special links.
// It renders files: Markdown keeps line breaks only with [markdown]
// ENABLE_HARD_LINE_BREAK, and its YAML front matter renders as a table.
//
// A document larger than [ui] MAX_DISPLAY_FILE_SIZE is shown as escaped plain
// text instead of being rendered.
func Render(typ Type, input any, urlPrefix string, metas map[string]string) []byte {
	return renderInput(typ, input, urlPrefix, metas, nil, conf.Markdown.EnableHardLineBreak, true)
}

// renderInput is Render, with commits resolved for Markdown, see
// ResolveCommits, hardWraps rendering every line break of Markdown as written,
// and file rendering front matter of Markdown files.
func renderInput(typ Type, input any, urlPrefix string, metas, commits map[string]string, hardWraps, file bool) []byte {
	var rawBytes []byte
	switch v := input.(type) {
	case []byte:
		rawBytes = v
	case string:
		rawBytes = []byte(v)
	default:
		panic(fmt.Sprintf("unrecognized input content type: %T", input))
	}

	if typ != TypeMarkdown && typ != TypeOrgMode {
		return rawBytes // Do nothing if syntax type is not recognized.
	}
	rawBytes = bytes.TrimPrefix(rawBytes, utf8BOM)
	return guardRender(rawBytes, func() []byte {
		return render(typ, rawBytes, urlPrefix, metas, commits, hardWraps, file)
	})
}

// guardRender returns body as escaped plain text when it is larger than [ui]
// MAX_DISPLAY_FILE_SIZE, and otherwise calls render within renderBudget.
func guardRender(body []byte, render func() []byte) []byte {
	if conf.UI.MaxDisplayFileSize > 0 && int64(len(body)) > conf.UI.MaxDisplayFileSize {
		return []byte("<pre>" + html.EscapeString(string(body)) + "</pre>")
	}
	if len(body) >= largeRenderSize {
		weight := int64(min(len(body), renderBudget))
		// Acquire only fails when its context is done, which never happens.
		_ = renderSemaphore.Acquire(context.Background(), weight)
		defer renderSemaphore.Release(weight)
	}
	return render()
}

func render(typ Type, rawBytes []byte, urlPrefix string, metas, commits map[string]string, hardWraps, file bool) []byte {
	urlPrefix = strings.TrimRight(strings.ReplaceAll(urlPrefix, " ", "%20"), "/")
	var rawHTML []byte
	switch typ {
	case TypeMarkdown:
		res := renderMarkdown(rawBytes, urlPrefix, metas, commits, hardWraps, file)
		if res.Sanitized {
			return res.HTML
		}
		if n := res.FrontMatterSize; n > 0 {
			// The HTML of front matter is safe, and sanitizing it would take
			// more than rendering it.
			return append(res.HTML[:n:n], SanitizeBytes(res.HTML[n:])...)
		}
		rawHTML = res.HTML
	case TypeOrgMode:
		rawHTML = RawOrgMode(rawBytes, urlPrefix)
		rawHTML = postProcessHTML(rawHTML, urlPrefix, metas)
	}
	return SanitizeBytes(rawHTML)
}
