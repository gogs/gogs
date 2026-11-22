package web

import (
	"bytes"
	"html"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/public"
)

// markAppLinks gives the links in rendered, sanitized HTML that lead to pages
// of the web app a "data-app-link" attribute, so the web app follows them
// without reloading the document. The sanitizer drops data attributes, so
// content cannot mark a link itself. repo is the repository the HTML was
// rendered for.
func markAppLinks(rendered []byte, repo *database.Repository) string {
	var b strings.Builder
	b.Grow(len(rendered))
	rest := rendered
	for {
		i := bytes.Index(rest, []byte("<a "))
		if i < 0 {
			break
		}
		// Sanitized attribute values have "<", ">" and quotes escaped, so the
		// tag ends at the first ">", and no value contains ` href="`.
		end := bytes.IndexByte(rest[i:], '>')
		if end < 0 {
			break
		}
		b.Write(rest[:i+len("<a")])
		tag := rest[i : i+end]
		if start := bytes.Index(tag, []byte(` href="`)); start >= 0 {
			href := tag[start+len(` href="`):]
			if n := bytes.IndexByte(href, '"'); n >= 0 && isAppLink(html.UnescapeString(string(href[:n])), repo) {
				b.WriteString(" data-app-link")
			}
		}
		rest = rest[i+len("<a"):]
	}
	b.Write(rest)
	return b.String()
}

// isAppLink reports whether href leads to a page of the web app on this site.
func isAppLink(href string, repo *database.Repository) bool {
	if rest, ok := strings.CutPrefix(href, conf.Server.ExternalURL); ok {
		href = conf.Server.Subpath + "/" + rest
	}
	if !strings.HasPrefix(href, "/") || strings.HasPrefix(href, "//") {
		return false
	}
	u, err := url.Parse(href)
	if err != nil {
		return false
	}
	// Browsers resolve dot segments of the whole path, which may leave the
	// subpath, before requesting it.
	p, ok := strings.CutPrefix(path.Clean(u.Path), conf.Server.Subpath)
	if !ok || p != "" && p[0] != '/' {
		return false
	}
	return isWebAppPage(path.Clean("/"+p), u.Query(), repo)
}

// isWebAppPage reports whether a GET request for p, a clean path without the
// subpath, reaches the web app, i.e., none of the static files or the routes
// mounted in newFlame before the web app serves it or redirects elsewhere. It
// must be kept in sync with those routes. The issues and the wiki of
// repositories other than repo, which the server may redirect to an external
// tracker or wiki, are not counted, except for single issues, whose page
// redirects itself.
func isWebAppPage(p string, query url.Values, repo *database.Repository) bool {
	if isStaticFile(p) {
		return false
	}
	segs := strings.Split(strings.Trim(p, "/"), "/")
	switch segs[0] {
	case "-", "api", "assets", "attachments", "healthcheck", "redirect", "robots.txt":
		return false
	}
	if len(segs) == 1 {
		return segs[0] != "explore" && !strings.HasSuffix(segs[0], ".keys")
	}
	if segs[0] == "user" && (segs[1] == "activate_email" || segs[1] == "email2user") {
		return false
	}

	// The rest are the paths of a repository, e.g., "/{owner}/{repo}/src/main".
	sub := segs[2:]
	if len(sub) == 0 {
		return query.Get("go-get") != "1"
	}
	last := sub[len(sub)-1]
	sameRepo := repo != nil && strings.EqualFold("/"+segs[0]+"/"+segs[1], strings.TrimPrefix(repo.Link(), conf.Server.Subpath))
	switch sub[0] {
	case "archive", "git-receive-pack", "git-upload-pack", "info", "raw", "tasks":
		return false
	case "commit", "pulls":
		return !strings.HasSuffix(last, ".diff") && !strings.HasSuffix(last, ".patch")
	case "compare":
		return !strings.HasSuffix(last, ".diff")
	case "issues":
		if len(sub) == 2 && isDigits(sub[1]) || len(sub) > 2 {
			return true
		}
		return sameRepo && repo.ExternalTrackerLink() == ""
	case "wiki":
		if len(sub) == 2 && !isWikiPageName(sub[1]) {
			return false
		}
		if len(sub) == 1 || len(sub) == 2 || len(sub) == 3 && sub[2] == "_edit" {
			return sameRepo && repo.ExternalWikiLink() == ""
		}
	}
	return true
}

func isDigits(s string) bool {
	return s != "" && strings.Trim(s, "0123456789") == ""
}

// isStaticFile reports whether p names a file or directory that the static
// file handlers of newFlame and the web app serve before any route. The
// public directory on disk, used with [server] LOAD_ASSETS_FROM_DISK, has the
// same files as the embedded one.
func isStaticFile(p string) bool {
	name := strings.TrimPrefix(p, "/")
	if name == "" {
		return false
	}
	if _, err := fs.Stat(public.Files, name); err == nil {
		return true
	}
	if _, err := fs.Stat(public.WebAssets, "dist/"+name); err == nil {
		return true
	}
	_, err := os.Stat(filepath.Join(conf.CustomDir(), "public", filepath.FromSlash(name)))
	return err == nil
}
