package urlx

import "strings"

// IsSameSite returns true if the URL path belongs to the same site.
func IsSameSite(rawURL string) bool {
	p := strings.NewReplacer(
		`\`, "/", "%5C", "/", "%5c", "/",
		"\t", "", "\n", "", "\r", "",
	).Replace(rawURL)
	return strings.HasPrefix(p, "/") && !strings.Contains(p, "//")
}
