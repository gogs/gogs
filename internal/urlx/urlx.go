package urlx

import "strings"

// IsSameSite returns true if the raw URL is a relative path that belongs to the
// same site, so a caller can safely redirect to it verbatim.
//
// The check is deliberately strict: any character that a browser would rewrite
// while resolving the Location header (a backslash in raw or percent-encoded
// form, or stripped whitespace) makes the URL not same-site, because the string
// that reaches the browser differs from the one validated here.
func IsSameSite(rawURL string) bool {
	if strings.ContainsAny(rawURL, "\\\t\n\r") ||
		strings.Contains(rawURL, "%5C") || strings.Contains(rawURL, "%5c") {
		return false
	}
	return strings.HasPrefix(rawURL, "/") && !strings.Contains(rawURL, "//")
}
