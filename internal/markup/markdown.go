package markup

import (
	"bytes"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"

	"github.com/russross/blackfriday/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/lazyregexp"
	"gogs.io/gogs/internal/tool"
)

// IsMarkdownFile reports whether name looks like a Markdown file based on its extension.
func IsMarkdownFile(name string) bool {
	extension := strings.ToLower(filepath.Ext(name))
	for _, ext := range conf.Markdown.FileExtensions {
		if strings.ToLower(ext) == extension {
			return true
		}
	}
	return false
}

// MarkdownRenderer is an extended version of the underlying Markdown render object.
type MarkdownRenderer struct {
	*blackfriday.HTMLRenderer
	urlPrefix string
}

var validLinksPattern = lazyregexp.New(`^[a-z][\w-]+://|^mailto:`)

// isLink reports whether link fits valid format.
func isLink(link []byte) bool {
	return validLinksPattern.Match(link)
}

// isAutoLink reports whether the link node was generated from an autolink
// (the child text matches the destination).
func isAutoLink(node *blackfriday.Node) bool {
	if node.FirstChild == nil {
		return false
	}
	return bytes.Equal(node.FirstChild.Literal, node.Destination)
}

func (r *MarkdownRenderer) RenderNode(w io.Writer, node *blackfriday.Node, entering bool) blackfriday.WalkStatus {
	switch node.Type {
	case blackfriday.Link:
		if isAutoLink(node) {
			return r.renderAutoLink(w, node, entering)
		}
		return r.renderLink(w, node, entering)

	case blackfriday.Item:
		if entering {
			return r.renderListItem(w, node)
		}
	}

	return r.HTMLRenderer.RenderNode(w, node, entering)
}

func (r *MarkdownRenderer) renderLink(w io.Writer, node *blackfriday.Node, entering bool) blackfriday.WalkStatus {
	if entering {
		dest := node.Destination
		if len(dest) > 0 && !isLink(dest) && dest[0] != '#' {
			node.Destination = []byte(path.Join(r.urlPrefix, string(dest)))
		}
	}
	return r.HTMLRenderer.RenderNode(w, node, entering)
}

func (r *MarkdownRenderer) renderAutoLink(w io.Writer, node *blackfriday.Node, entering bool) blackfriday.WalkStatus {
	if !entering {
		return r.HTMLRenderer.RenderNode(w, node, entering)
	}

	link := node.Destination

	if bytes.HasPrefix(link, []byte(conf.Server.ExternalURL)) {
		m := CommitPattern.Find(link)
		if m != nil {
			m = bytes.TrimSpace(m)
			i := bytes.Index(m, []byte("commit/"))
			j := bytes.Index(m, []byte("#"))
			if j == -1 {
				j = len(m)
			}
			_, _ = fmt.Fprintf(w, ` <code><a href="%s">%s</a></code>`, m, tool.ShortSHA1(string(m[i+7:j])))
			return blackfriday.SkipChildren
		}

		m = IssueFullPattern.Find(link)
		if m != nil {
			m = bytes.TrimSpace(m)
			i := bytes.Index(m, []byte("issues/"))
			j := bytes.Index(m, []byte("#"))
			if j == -1 {
				j = len(m)
			}

			index := string(m[i+7 : j])
			fullRepoURL := conf.Server.ExternalURL + strings.TrimPrefix(r.urlPrefix, "/")
			var result string
			if strings.HasPrefix(string(m), fullRepoURL) {
				result = fmt.Sprintf(`<a href="%s">#%s</a>`, m, index)
			} else {
				repo := string(m[len(conf.Server.ExternalURL) : i-1])
				result = fmt.Sprintf(`<a href="%s">%s#%s</a>`, m, repo, index)
			}
			_, _ = io.WriteString(w, result)
			return blackfriday.SkipChildren
		}
	}

	return r.HTMLRenderer.RenderNode(w, node, entering)
}

func (r *MarkdownRenderer) renderListItem(w io.Writer, node *blackfriday.Node) blackfriday.WalkStatus {
	if node.FirstChild != nil && node.FirstChild.Type == blackfriday.Paragraph && node.FirstChild.FirstChild != nil {
		textNode := node.FirstChild.FirstChild
		if textNode.Type == blackfriday.Text {
			text := textNode.Literal
			switch {
			case bytes.HasPrefix(text, []byte("[ ] ")):
				textNode.Literal = append([]byte(`<input type="checkbox" disabled="" />`), text[3:]...)
			case bytes.HasPrefix(text, []byte("[x] ")):
				textNode.Literal = append([]byte(`<input type="checkbox" disabled="" checked="" />`), text[3:]...)
			}
		}
	}
	return r.HTMLRenderer.RenderNode(w, node, true)
}

// RawMarkdown renders content in Markdown syntax to HTML without handling special links.
func RawMarkdown(body []byte, urlPrefix string) []byte {
	var htmlFlags blackfriday.HTMLFlags
	htmlFlags |= blackfriday.SkipHTML

	if conf.Smartypants.Enabled {
		htmlFlags |= blackfriday.Smartypants
		if conf.Smartypants.Fractions {
			htmlFlags |= blackfriday.SmartypantsFractions
		}
		if conf.Smartypants.Dashes {
			htmlFlags |= blackfriday.SmartypantsDashes
		}
		if conf.Smartypants.LatexDashes {
			htmlFlags |= blackfriday.SmartypantsLatexDashes
		}
		if conf.Smartypants.AngledQuotes {
			htmlFlags |= blackfriday.SmartypantsAngledQuotes
		}
	}

	renderer := &MarkdownRenderer{
		HTMLRenderer: blackfriday.NewHTMLRenderer(blackfriday.HTMLRendererParameters{Flags: htmlFlags}),
		urlPrefix:    urlPrefix,
	}

	var extensions blackfriday.Extensions
	extensions |= blackfriday.NoIntraEmphasis
	extensions |= blackfriday.Tables
	extensions |= blackfriday.FencedCode
	extensions |= blackfriday.Autolink
	extensions |= blackfriday.Strikethrough
	extensions |= blackfriday.SpaceHeadings
	extensions |= blackfriday.NoEmptyLineBeforeBlock

	if conf.Markdown.EnableHardLineBreak {
		extensions |= blackfriday.HardLineBreak
	}

	return blackfriday.Run(body, blackfriday.WithRenderer(renderer), blackfriday.WithExtensions(extensions))
}

// Markdown takes a string or []byte and renders to HTML in Markdown syntax with special links.
func Markdown(input any, urlPrefix string, metas map[string]string) []byte {
	return Render(TypeMarkdown, input, urlPrefix, metas)
}
