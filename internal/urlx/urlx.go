package urlx

import "strings"

// IsSameSite returns true if the URL path belongs to the same site.
func IsSameSite(rawURL string) bool {
	if strings.ContainsAny(rawURL, "\\\t\n\r") ||
		strings.Contains(rawURL, "%5C") || strings.Contains(rawURL, "%5c") {
		return false
	}
	return strings.HasPrefix(rawURL, "/") && !strings.Contains(rawURL, "//")
}
