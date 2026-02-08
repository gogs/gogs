package markup

import (
	"bytes"
	"fmt"
	"html"
	"log"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	goldmarkhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

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

var validLinksPattern = lazyregexp.New(`^[a-z][\w-]+://|^mailto:`)
var linkifyURLRegexp = regexp.MustCompile(`^(?:http|https|ftp)://[-a-zA-Z0-9@:%._+~#=]{1,256}(?:\.[a-z]+)?(?::\d+)?(?:[/#?][-a-zA-Z0-9@:%_+.~#$!?&/=();,'\^{}\[\]` + "`" + `]*)?`)

func isLink(link []byte) bool {
	return validLinksPattern.Match(link)
}

type linkTransformer struct {
	urlPrefix string
}

func (t *linkTransformer) Transform(node *ast.Document, reader text.Reader, _ parser.Context) {
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if link, ok := n.(*ast.Link); ok {
			dest := link.Destination
			if len(dest) > 0 && !isLink(dest) && dest[0] != '#' {
				link.Destination = []byte(path.Join(t.urlPrefix, string(dest)))
			}
		}
		return ast.WalkContinue, nil
	})
}

type gogsRenderer struct {
	urlPrefix string
}

func (r *gogsRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindAutoLink, r.renderAutoLink)
}

func (r *gogsRenderer) renderAutoLink(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.AutoLink)
	if !entering {
		return ast.WalkContinue, nil
	}

	if n.AutoLinkType != ast.AutoLinkURL {
		url := n.URL(source)
		escaped := html.EscapeString(string(url))
		_, _ = fmt.Fprintf(w, `<a href="mailto:%s">%s</a>`, escaped, escaped)
		return ast.WalkContinue, nil
	}

	link := n.URL(source)

	if bytes.HasPrefix(link, []byte(conf.Server.ExternalURL)) {
		m := CommitPattern.Find(link)
		if m != nil {
			m = bytes.TrimSpace(m)
			i := bytes.Index(m, []byte("commit/"))
			j := bytes.Index(m, []byte("#"))
			if j == -1 {
				j = len(m)
			}
			escapedURL := html.EscapeString(string(m))
			_, _ = fmt.Fprintf(w, ` <code><a href="%s">%s</a></code>`, escapedURL, tool.ShortSHA1(string(m[i+7:j])))
			return ast.WalkContinue, nil
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
			escapedURL := html.EscapeString(string(m))
			fullRepoURL := conf.Server.ExternalURL + strings.TrimPrefix(r.urlPrefix, "/")
			var href string
			if strings.HasPrefix(string(m), fullRepoURL) {
				href = fmt.Sprintf(`<a href="%s">#%s</a>`, escapedURL, html.EscapeString(index))
			} else {
				repo := html.EscapeString(string(m[len(conf.Server.ExternalURL) : i-1]))
				href = fmt.Sprintf(`<a href="%s">%s#%s</a>`, escapedURL, repo, html.EscapeString(index))
			}
			_, _ = w.WriteString(href)
			return ast.WalkContinue, nil
		}
	}

	escapedLink := html.EscapeString(string(link))
	_, _ = fmt.Fprintf(w, `<a href="%s">%s</a>`, escapedLink, escapedLink)
	return ast.WalkContinue, nil
}

// RawMarkdown renders content in Markdown syntax to HTML without handling special links.
func RawMarkdown(body []byte, urlPrefix string) []byte {
	extensions := []goldmark.Extender{
		extension.Table,
		extension.Strikethrough,
		extension.TaskList,
		extension.NewLinkify(extension.WithLinkifyURLRegexp(linkifyURLRegexp)),
	}

	if conf.Smartypants.Enabled {
		extensions = append(extensions, extension.Typographer)
	}

	rendererOpts := []renderer.Option{
		goldmarkhtml.WithUnsafe(),
		renderer.WithNodeRenderers(
			util.Prioritized(&gogsRenderer{urlPrefix: urlPrefix}, 0),
		),
	}

	if conf.Markdown.EnableHardLineBreak {
		rendererOpts = append(rendererOpts, goldmarkhtml.WithHardWraps())
	}

	md := goldmark.New(
		goldmark.WithExtensions(extensions...),
		goldmark.WithParserOptions(
			parser.WithASTTransformers(
				util.Prioritized(&linkTransformer{urlPrefix: urlPrefix}, 0),
			),
		),
		goldmark.WithRendererOptions(rendererOpts...),
	)

	var buf bytes.Buffer
	if err := md.Convert(body, &buf); err != nil {
		log.Printf("markup: failed to convert Markdown: %v", err)
		return nil
	}
	return buf.Bytes()
}

// Markdown takes a string or []byte and renders to HTML in Markdown syntax with special links.
func Markdown(input any, urlPrefix string, metas map[string]string) []byte {
	return Render(TypeMarkdown, input, urlPrefix, metas)
}
