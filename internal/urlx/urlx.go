package urlx

import (
	"net/url"
	"strings"
)

// IsSameSite returns true if the URL path belongs to the same site.
//
// Browsers normalize backslashes to forward slashes on the Location header,
// so inputs like "/a/../\example.com" become "/a/..//example.com" and then
// resolve to the cross-origin "//example.com". Apply the same backslash
// normalization first, then require the result to parse as a relative URL
// (no scheme, no host) whose path starts with a single "/" and contains no
// "//" runs that traversal could later expose.
func IsSameSite(rawURL string) bool {
	u, err := url.Parse(strings.ReplaceAll(rawURL, `\`, "/"))
	if err != nil || u.Scheme != "" || u.Host != "" {
		return false
	}
	p := strings.ReplaceAll(u.Path, `\`, "/")
	return strings.HasPrefix(p, "/") && !strings.Contains(p, "//")
}
