package urlx

// IsSameSite returns true if the URL path belongs to the same site.
func IsSameSite(url string) bool {
	return len(url) >= 2 && url[0] == '/' && url[1] != '/' && url[1] != '\\'
}
