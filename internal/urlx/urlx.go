package urlx

import (
	"net/url"
	"strings"
)

// IsSameSite returns true if the URL path belongs to the same site.
func IsSameSite(rawURL string) bool {
	if strings.ContainsAny(rawURL, "\\\t\n\r") ||
		strings.Contains(rawURL, "%5C") || strings.Contains(rawURL, "%5c") {
		return false
	}
	return strings.HasPrefix(rawURL, "/") && !strings.Contains(rawURL, "//")
}

// IsHTTP returns true if the URL is an absolute HTTP or HTTPS URL, which is
// safe to send browsers to, unlike e.g., a "javascript:" URL.
func IsHTTP(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
