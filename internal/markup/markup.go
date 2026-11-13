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
	"gogs.io/gogs/internal/lazyregexp"
	"gogs.io/gogs/internal/tool"
)

// IsReadmeFile reports whether name looks like a README file based on its extension.
func IsReadmeFile(name string) bool {
	return strings.HasPrefix(strings.ToLower(name), "readme")
}

// IsIPythonNotebook reports whether name looks like a IPython notebook based on its extension.
func IsIPythonNotebook(name string) bool {
	return strings.HasSuffix(name, ".ipynb")
}

const (
	IssueNameStyleNumeric      = "numeric"
	IssueNameStyleAlphanumeric = "alphanumeric"
)

var (
	// MentionPattern matches string that mentions someone, e.g. @Unknwon
	MentionPattern = lazyregexp.New(`(\s|^|\W)@[0-9a-zA-Z-_\.]+`)

	// CommitPattern matches link to certain commit with or without trailing hash,
	// e.g. https://try.gogs.io/gogs/gogs/commit/d8a994ef243349f321568f9e36d5c3f444b99cae#diff-2
	CommitPattern = lazyregexp.New(`(\s|^)https?.*commit/[0-9a-zA-Z]+(#+[0-9a-zA-Z-]*)?`)

	// IssueFullPattern matches link to an issue with or without trailing hash,
	// e.g. https://try.gogs.io/gogs/gogs/issues/4#issue-685
	IssueFullPattern = lazyregexp.New(`(\s|^)https?.*issues/[0-9]+(#+[0-9a-zA-Z-]*)?`)
	// IssueNumericPattern matches string that references to a numeric issue, e.g. #1287
	IssueNumericPattern = lazyregexp.New(`( |^|\(|\[)#[0-9]+\b`)
	// IssueAlphanumericPattern matches string that references to an alphanumeric issue, e.g. ABC-1234
	IssueAlphanumericPattern = lazyregexp.New(`( |^|\(|\[)[A-Z]{1,10}-[1-9][0-9]*\b`)
	// CrossReferenceIssueNumericPattern matches string that references a numeric issue in a difference repository
	// e.g. gogs/gogs#12345
	CrossReferenceIssueNumericPattern = lazyregexp.New(`( |^)[0-9a-zA-Z-_\.]+/[0-9a-zA-Z-_\.]+#[0-9]+\b`)

	// Sha1CurrentPattern matches string that represents a commit SHA, e.g. d8a994ef243349f321568f9e36d5c3f444b99cae
	// FIXME: this pattern matches pure numbers as well, right now we do a hack to check in RenderSha1CurrentPattern by converting string to a number.
	// Only full SHA-256 commit IDs are longer than 40, so other hex strings, e.g.,
	// checksums of 41 to 63 characters, are not mistaken for commit IDs. Those of
	// 64 characters are only linked in SHA-256 repositories.
	Sha1CurrentPattern = lazyregexp.New(`\b(?:[0-9a-f]{7,40}|[0-9a-f]{64})\b`)
)

// FindAllMentions matches mention patterns in given content
// and returns a list of found user names without @ prefix.
func FindAllMentions(content string) []string {
	mentions := MentionPattern.FindAllString(content, -1)
	for i := range mentions {
		mentions[i] = mentions[i][strings.Index(mentions[i], "@")+1:] // Strip @ character
	}
	return mentions
}

// cutoutVerbosePrefix cutouts URL prefix including sub-path to
// return a clean unified string of request URL path.
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

// expand substitutes "{key}" placeholders in template with values from match.
func expand(template string, match map[string]string) string {
	pairs := make([]string, 0, len(match)*2)
	for k, v := range match {
		pairs = append(pairs, "{"+k+"}", v)
	}
	return strings.NewReplacer(pairs...).Replace(template)
}

// linkSpan is a reference found in text and the link that replaces it.
type linkSpan struct {
	start, end int
	link       []byte
}

// findIssueIndexes returns the issue references in text, outside the spans of
// skip.
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
		var link []byte
		if match == nil {
			link = fmt.Appendf(nil, `<a href="%s/issues/%s">%s</a>`, urlPrefix, m[1:], m)
		} else {
			if alphanumeric {
				match["index"] = string(m)
			} else {
				match["index"] = string(m[1:])
			}
			link = fmt.Appendf(nil, `<a href="%s">%s</a>`, html.EscapeString(expand(metas["format"], match)), m)
		}
		spans = append(spans, linkSpan{start: start, end: loc[1], link: link})
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
		for i < len(spans) && spans[i].end <= s.start {
			merged = append(merged, spans[i])
			i++
		}
		if i < len(spans) && spans[i].start < s.end {
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
	out := make([]byte, 0, len(text)+len(spans)*48)
	last := 0
	for _, s := range spans {
		out = append(out, text[last:s.start]...)
		out = append(out, s.link...)
		last = s.end
	}
	return append(out, text[last:]...)
}

// RenderIssueIndexPattern renders issue indexes to corresponding links.
func RenderIssueIndexPattern(rawBytes []byte, urlPrefix string, metas map[string]string) []byte {
	return replaceSpans(rawBytes, findIssueIndexes(rawBytes, urlPrefix, metas, nil))
}

// findCrossReferences returns the references to issues of other repositories
// in text, outside the spans of skip.
func findCrossReferences(text []byte, skip []linkSpan) []linkSpan {
	var spans []linkSpan
	for _, loc := range CrossReferenceIssueNumericPattern.FindAllIndex(text, -1) {
		start := loc[0]
		if c := text[start]; c == ' ' || c == '(' {
			start++
		}
		m := text[start:loc[1]]
		delim := bytes.IndexByte(m, '#')
		link := fmt.Appendf(nil, `<a href="%s%s/issues/%s">%s</a>`, conf.Server.ExternalURL, m[:delim], m[delim+1:], m)
		spans = append(spans, linkSpan{start: start, end: loc[1], link: link})
	}
	return mergeSpans(skip, spans)
}

// RenderCrossReferenceIssueIndexPattern renders issue indexes from other repositories to corresponding links.
func RenderCrossReferenceIssueIndexPattern(rawBytes []byte, _ string, _ map[string]string) []byte {
	return replaceSpans(rawBytes, findCrossReferences(rawBytes, nil))
}

// findSha1s returns the commit IDs in text, outside the spans of skip.
func findSha1s(text []byte, metas map[string]string, skip []linkSpan) []linkSpan {
	var spans []linkSpan
	for _, loc := range Sha1CurrentPattern.FindAllIndex(text, -1) {
		m := string(text[loc[0]:loc[1]])
		if v, _ := strconv.Atoi(m); v > 0 {
			continue
		} else if len(m) == 64 && metas["objectFormat"] != "sha256" {
			continue
		}
		link := fmt.Appendf(nil, `<a href="%s/commit/%s"><code>%s</code></a>`, metas["repoLink"], m, tool.ShortSHA1(m))
		spans = append(spans, linkSpan{start: loc[0], end: loc[1], link: link})
	}
	return mergeSpans(skip, spans)
}

// RenderSha1CurrentPattern renders commit IDs to corresponding links that assumes
// in the same repository, i.e., metas["repoLink"]. Hex strings of 64 characters
// are only rendered when metas["objectFormat"] is "sha256".
func RenderSha1CurrentPattern(rawBytes []byte, metas map[string]string) []byte {
	return replaceSpans(rawBytes, findSha1s(rawBytes, metas, nil))
}

// RenderSpecialLink renders mentions, indexes and commit IDs to corresponding links.
// It replaces each reference where it is found, in a single pass, and a
// reference that overlaps an earlier kind of reference is left as text.
func RenderSpecialLink(rawBytes []byte, urlPrefix string, metas map[string]string) []byte {
	var spans []linkSpan
	for _, loc := range MentionPattern.Regexp().FindAllSubmatchIndex(rawBytes, -1) {
		// The mention starts after the character before the "@" that the
		// pattern requires.
		start := loc[3]
		m := rawBytes[start:loc[1]]
		spans = append(spans, linkSpan{start: start, end: loc[1], link: fmt.Appendf(nil, `<a href="%s/%s">%s</a>`, conf.Server.Subpath, m[1:], m)})
	}
	spans = findIssueIndexes(rawBytes, urlPrefix, metas, spans)
	spans = findCrossReferences(rawBytes, spans)
	spans = findSha1s(rawBytes, metas, spans)
	return replaceSpans(rawBytes, spans)
}

var (
	leftAngleBracket  = []byte("</")
	rightAngleBracket = []byte(">")
)

var noEndTags = []string{"input", "br", "hr", "img"}

// wrapImgWithLink warps link to standalone <img> tags.
func wrapImgWithLink(urlPrefix string, buf *bytes.Buffer, token html.Token) {
	// Extract "src" and "alt" attributes
	var src, alt string
	for i := range token.Attr {
		switch token.Attr[i].Key {
		case "src":
			src = token.Attr[i].Val
		case "alt":
			alt = token.Attr[i].Val
		}
	}

	// Skip in case the "src" is empty
	if src == "" {
		buf.WriteString(token.String())
		return
	}

	// Skip in case the "src" is data url
	if strings.HasPrefix(src, "data:") {
		buf.WriteString(token.String())
		return
	}

	// Prepend repository base URL for internal links
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
	startTags := make([]string, 0, 5)
	buf := bytes.NewBuffer(nil)
	tokenizer := html.NewTokenizer(bytes.NewReader(rawHTML))

outerLoop:
	for html.ErrorToken != tokenizer.Next() {
		token := tokenizer.Token()
		switch token.Type {
		case html.TextToken:
			buf.Write(RenderSpecialLink([]byte(token.String()), urlPrefix, metas))

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

					// Copy the token to the output verbatim
					buf.WriteString(token.String())

					// Stack number doesn't increase for tags without end tags.
					if token.Type == html.StartTagToken && !slices.Contains(noEndTags, token.Data) {
						stackNum++
					}

					// If this is the close tag to the outer-most, we are done
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
	largeRenderSize = 256 << 10
	// renderBudget is the total input size of large documents rendered at the
	// same time. Rendering takes memory proportional to the input, so this
	// bounds the memory that concurrent large renders use. A document larger
	// than the budget waits until it can be rendered alone.
	renderBudget = 32 << 20
)

var renderSemaphore = semaphore.NewWeighted(renderBudget)

// utf8BOM is the byte order mark that some editors write at the start of
// files. It is not part of the content.
var utf8BOM = []byte("\ufeff")

// Render takes a string or []byte and renders to sanitized HTML in given type of syntax with special links.
//
// A document larger than [ui] MAX_DISPLAY_FILE_SIZE is shown as escaped plain
// text instead of being rendered.
func Render(typ Type, input any, urlPrefix string, metas map[string]string) []byte {
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
		return rawBytes // Do nothing if syntax type is not recognized
	}
	rawBytes = bytes.TrimPrefix(rawBytes, utf8BOM)
	return guardRender(rawBytes, func() []byte {
		return render(typ, rawBytes, urlPrefix, metas)
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

func render(typ Type, rawBytes []byte, urlPrefix string, metas map[string]string) []byte {
	urlPrefix = strings.TrimRight(strings.ReplaceAll(urlPrefix, " ", "%20"), "/")
	var rawHTML []byte
	switch typ {
	case TypeMarkdown:
		var sanitized bool
		rawHTML, sanitized = renderMarkdown(rawBytes, urlPrefix, metas)
		if sanitized {
			return rawHTML
		}
	case TypeOrgMode:
		rawHTML = RawOrgMode(rawBytes, urlPrefix)
		rawHTML = postProcessHTML(rawHTML, urlPrefix, metas)
	}
	return SanitizeBytes(rawHTML)
}
