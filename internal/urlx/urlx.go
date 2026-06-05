package urlx

import (
	"net/url"
	"path"
	"strings"
)

// IsSameSite returns true if the URL path belongs to the same site.
//
// Browsers normalize backslashes to forward slashes before resolving a URL,
// so any backslash in the input could turn what looks like a same-site path
// into a protocol-relative cross-origin URL (e.g. "/a/../\example.com" becomes
// "//example.com" after normalization). Reject backslashes outright and parse
// the input to ensure it has no scheme or host, then resolve "." and ".."
// segments so traversal cannot escape the leading "/".
func IsSameSite(rawURL string) bool {
	if len(rawURL) < 2 || rawURL[0] != '/' || rawURL[1] == '/' {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if u.Scheme != "" || u.Host != "" {
		return false
	}
	if strings.ContainsRune(rawURL, '\\') || strings.ContainsRune(u.Path, '\\') {
		return false
	}
	cleaned := path.Clean(u.Path)
	return strings.HasPrefix(cleaned, "/") && !strings.HasPrefix(cleaned, "//")
}
