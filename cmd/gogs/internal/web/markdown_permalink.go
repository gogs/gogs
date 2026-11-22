package web

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"

	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/lazyregexp"
	"gogs.io/gogs/internal/mimex"
	"gogs.io/gogs/internal/repox"
)

const (
	// permalinkMaxLines is how many lines an embedded permalink shows at most.
	permalinkMaxLines = 50
	// permalinkMaxEmbeds is how many permalinks of one document are looked up
	// at most, and permalinkMaxFileSize the largest file they embed lines of,
	// so that rendering a document reads a bounded amount of Git data.
	permalinkMaxEmbeds   = 10
	permalinkMaxFileSize = 512 * 1024
)

var (
	// renderedLinkPattern matches a link as rendered Markdown has it.
	renderedLinkPattern = lazyregexp.New(`<a href="([^"]*)"[^>]*>([^<]*)</a>`)
	// permalinkPattern matches what follows "/src/" in the permalink of
	// lines of a file: the full commit ID, the path and the lines.
	permalinkPattern = lazyregexp.New(`^([0-9a-f]{40}|[0-9a-f]{64})/([^#?]+)#L(\d+)(?:-L(\d+))?$`)
)

// embedPermalinks shows the lines of permalinks to files of the repository at
// a commit, e.g., ".../src/<commit ID>/main.go#L10-L20", below the link. Only
// a bare link on a line of its own embeds, and only when its lines exist.
// Links to branches, other repositories or other instances, and links with
// text, stay links.
//
// The embed is an element with the data attributes the client renders it
// from, which the sanitizer drops from user content, so content cannot forge
// one. Viewers who cannot read the code of the repository, e.g., guests of the
// issues of a private repository, see only the links.
func embedPermalinks(ctx context.Context, logger *logx.Logger, ownerName, repoName string, canReadCode bool, rendered string) string {
	prefix := conf.Server.ExternalURL + ownerName + "/" + repoName + "/src/"
	if !canReadCode || !strings.Contains(strings.ToLower(rendered), strings.ToLower(prefix)) {
		return rendered
	}

	var gitRepo *git.Repository
	lookups := 0
	var b strings.Builder
	last := 0
	// reopen is whether the paragraph of the last embed goes on after it.
	reopen := false
	for _, m := range renderedLinkPattern.Regexp().FindAllStringSubmatchIndex(rendered, -1) {
		start, end := m[0], m[1]
		href := html.UnescapeString(rendered[m[2]:m[3]])
		if href != html.UnescapeString(rendered[m[4]:m[5]]) {
			continue
		}
		before, after := rendered[:start], rendered[end:]
		var opensParagraph, closesParagraph bool
		switch {
		case strings.HasSuffix(before, "<p>"):
		case strings.HasSuffix(before, "<br>\n"):
			opensParagraph = true
		default:
			continue
		}
		switch {
		case strings.HasPrefix(after, "</p>"):
		case strings.HasPrefix(after, "<br>\n"):
			closesParagraph = true
		default:
			continue
		}
		if len(href) <= len(prefix) || !strings.EqualFold(href[:len(prefix)], prefix) {
			continue
		}
		if lookups == permalinkMaxEmbeds {
			break
		}
		lookups++

		if gitRepo == nil {
			var err error
			gitRepo, err = git.Open(repox.RepositoryPath(ownerName, repoName))
			if err != nil {
				logger.Warn("Failed to open repository to embed permalinks", "repo", ownerName+"/"+repoName, "error", err)
				return rendered
			}
		}
		embed := permalinkEmbed(ctx, gitRepo, href, href[len(prefix):])
		if embed == "" {
			continue
		}

		// The link is the whole paragraph, or one of its lines. Then the
		// paragraph closes before the embed and opens again after it.
		lineStart := start - len("<p>")
		if opensParagraph {
			lineStart = start - len("<br>\n")
		}
		lineEnd := end + len("</p>")
		if strings.HasPrefix(rendered[lineEnd:], "\n") {
			lineEnd++
		}
		if closesParagraph {
			lineEnd = end + len("<br>\n")
		}
		if lineStart < last {
			// The previous embed took the line break before this link, and
			// the paragraph has not opened again.
			lineStart = last
			opensParagraph = false
			reopen = false
		}
		if reopen {
			b.WriteString("<p>")
		}
		b.WriteString(rendered[last:lineStart])
		if opensParagraph {
			b.WriteString("</p>\n")
		}
		b.WriteString(embed)
		reopen = closesParagraph
		last = lineEnd
	}
	if last == 0 {
		return rendered
	}
	if reopen {
		b.WriteString("<p>")
	}
	b.WriteString(rendered[last:])
	return b.String()
}

// permalinkEmbed returns the embed of the lines that rest, what follows
// "/src/" in the permalink, names. It returns an empty string when they do
// not exist or are not text.
func permalinkEmbed(ctx context.Context, gitRepo *git.Repository, href, rest string) string {
	m := permalinkPattern.FindStringSubmatch(rest)
	if m == nil {
		return ""
	}
	commitID := m[1]
	treePath, err := url.PathUnescape(m[2])
	if err != nil {
		return ""
	}
	first, err := strconv.Atoi(m[3])
	if err != nil || first < 1 {
		return ""
	}
	last := first
	if m[4] != "" {
		if last, err = strconv.Atoi(m[4]); err != nil {
			return ""
		}
	}
	if last < first {
		first, last = last, first
	}
	if first < 1 {
		return ""
	}

	commit, err := gitRepo.CatFileCommit(ctx, commitID)
	if err != nil {
		return ""
	}
	entry, err := commit.TreeEntry(ctx, treePath)
	if err != nil || !entry.IsBlob() {
		return ""
	}
	blob := entry.Blob()
	if size := blob.Size(ctx); size >= conf.UI.MaxDisplayFileSize || size > permalinkMaxFileSize {
		return ""
	}
	p, err := blob.Bytes(ctx)
	if err != nil || !mimex.IsText(p) {
		return ""
	}
	lines := strings.Split(strings.TrimSuffix(string(p), "\n"), "\n")
	if last > len(lines) {
		return ""
	}
	last = min(last, first+permalinkMaxLines-1)

	return fmt.Sprintf(
		`<div data-permalink="%s" data-path="%s" data-commit="%s" data-start="%d" data-end="%d"><pre><code>%s</code></pre></div>`+"\n",
		html.EscapeString(href),
		html.EscapeString(treePath),
		commitID,
		first,
		last,
		html.EscapeString(strings.Join(lines[first-1:last], "\n")),
	)
}
