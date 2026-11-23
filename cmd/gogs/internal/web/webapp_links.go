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
	"sync"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/public"
)

// markAppLinks gives the links in rendered, sanitized HTML that lead to pages
// of the web app a "data-app-link" attribute, so the web app follows them
// without reloading the document. The sanitizer drops data attributes, so
// content cannot mark a link itself. repo is the repository the HTML was
// rendered for, and o has the external URL and the subpath of the site.
func markAppLinks(o markup.Options, rendered []byte, repo *database.Repository) string {
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
			if n := bytes.IndexByte(href, '"'); n >= 0 && isAppLink(o, html.UnescapeString(string(href[:n])), repo) {
				b.WriteString(" data-app-link")
			}
		}
		rest = rest[i+len("<a"):]
	}
	b.Write(rest)
	return b.String()
}

// isAppLink reports whether href leads to a page of the web app on the site of
// o.
func isAppLink(o markup.Options, href string, repo *database.Repository) bool {
	if rest, ok := strings.CutPrefix(href, o.ExternalURL); ok {
		href = o.Subpath + "/" + rest
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
	p, ok := strings.CutPrefix(path.Clean(u.Path), o.Subpath)
	if !ok || p != "" && p[0] != '/' {
		return false
	}
	return isWebAppPage(path.Clean("/"+p), u.RawQuery, repo)
}

// isWebAppPage reports whether a GET request for p, a clean path without the
// subpath, with the query rawQuery reaches the web app, i.e., none of the
// static files or the routes mounted in newFlame before the web app serves it
// or redirects elsewhere. It must be kept in sync with those routes. The
// issues and the wiki of repositories other than repo, which the server may
// redirect to an external tracker or wiki, are not counted, except for single
// issues, whose page redirects itself.
func isWebAppPage(p, rawQuery string, repo *database.Repository) bool {
	segs := strings.Split(strings.Trim(p, "/"), "/")
	if staticNames()[segs[0]] {
		return false
	}
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
		query, _ := url.ParseQuery(rawQuery)
		return query.Get("go-get") != "1"
	}
	last := sub[len(sub)-1]
	sameRepo := repo != nil && strings.EqualFold("/"+segs[0]+"/"+segs[1], repo.RelLink())
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

// staticNames returns the names of the files and directories at the top of
// what the static file handlers of newFlame and the web app serve before any
// route. A path under one of them is not counted as a page, even when no such
// file exists, which only makes the link load the document. The public
// directory on disk, used with [server] LOAD_ASSETS_FROM_DISK, has the same
// files as the embedded one. Changes to the custom public directory are seen
// after a restart.
var staticNames = sync.OnceValue(func() map[string]bool {
	names := make(map[string]bool)
	add := func(entries []fs.DirEntry, _ error) {
		for _, e := range entries {
			names[e.Name()] = true
		}
	}
	add(fs.ReadDir(public.Files, "."))
	add(fs.ReadDir(public.WebAssets, "dist"))
	add(os.ReadDir(filepath.Join(conf.CustomDir(), "public")))
	return names
})
