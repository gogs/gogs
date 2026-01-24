package urlutil

// IsSameSite returns true if the URL path belongs to the same site, false otherwise.
// False: //url, http://url, /\url
// True: /url
func IsSameSite(url string) bool {
	return len(url) >= 2 && url[0] == '/' && url[1] != '/' && url[1] != '\\'
}
