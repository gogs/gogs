package urlx

import "strings"

// IsSameSite returns true if the URL path belongs to the same site.
//
// Browsers normalize backslashes (including percent-encoded ones) to forward
// slashes on the Location header, so inputs like "/a/../\example.com" become
// "/a/..//example.com" and then resolve to the cross-origin "//example.com".
// Mirror that normalization, then require a single leading "/" with no "//"
// runs that traversal could later expose.
func IsSameSite(rawURL string) bool {
	r := strings.NewReplacer(`\`, "/", "%5C", "/", "%5c", "/")
	p := r.Replace(rawURL)
	return strings.HasPrefix(p, "/") && !strings.Contains(p, "//")
}
