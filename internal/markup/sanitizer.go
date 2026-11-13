package markup

import (
	"net/url"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/microcosm-cc/bluemonday"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/lazyregexp"
)

// Sanitizer is a protection wrapper of *bluemonday.Policy which does not allow
// any modification to the underlying policies once it's been created.
type Sanitizer struct {
	policy *bluemonday.Policy
	init   sync.Once
	ready  atomic.Bool
	// schemes holds the URL schemes the policy allows without checks.
	schemes map[string]bool
}

var sanitizer = &Sanitizer{
	policy: bluemonday.UGCPolicy(),
}

// codeClassPattern matches the allowed classes of code elements.
var codeClassPattern = lazyregexp.New(`^language-[\w.+#-]+$`)

// urlSchemePolicies are the URL schemes that are allowed only for the URLs
// that pass their checks.
var urlSchemePolicies = map[string]func(*url.URL) bool{
	// Only allow data URIs with safe image MIME types to prevent XSS via
	// "data:text/html" payloads.
	"data": IsSafeDataURI,
}

// NewSanitizer initializes sanitizer with allowed attributes based on settings.
// Multiple calls to this function will only create one instance of Sanitizer during
// entire application lifecycle.
func NewSanitizer() {
	sanitizer.init.Do(func() {
		// We only want to allow HighlightJS specific classes for code blocks
		sanitizer.policy.AllowAttrs("class").Matching(codeClassPattern.Regexp()).OnElements("code")

		// Checkboxes
		sanitizer.policy.AllowAttrs("type").Matching(lazyregexp.New(`^checkbox$`).Regexp()).OnElements("input")
		sanitizer.policy.AllowAttrs("checked", "disabled").OnElements("input")

		// Alerts
		sanitizer.policy.AllowAttrs("class").Matching(lazyregexp.New(`^markdown-alert markdown-alert-(note|tip|important|warning|caution)$`).Regexp()).OnElements("div")
		sanitizer.policy.AllowAttrs("class").Matching(lazyregexp.New(`^markdown-alert-title$`).Regexp()).OnElements("p")

		// Start numbers of ordered lists
		sanitizer.policy.AllowAttrs("start").Matching(lazyregexp.New(`^[0-9]+$`).Regexp()).OnElements("ol")

		// Task lists
		sanitizer.policy.AllowAttrs("class").Matching(lazyregexp.New(`^contains-task-list$`).Regexp()).OnElements("ul", "ol")
		sanitizer.policy.AllowAttrs("class").Matching(lazyregexp.New(`^task-list-item$`).Regexp()).OnElements("li")
		sanitizer.policy.AllowAttrs("data-source-lines").Matching(lazyregexp.New(`^[0-9]+-[0-9]+$`).Regexp()).OnElements("li")

		// Footnotes
		sanitizer.policy.AllowAttrs("class").Matching(lazyregexp.New(`^footnotes$`).Regexp()).OnElements("div")
		sanitizer.policy.AllowAttrs("class").Matching(lazyregexp.New(`^footnote-(ref|backref)$`).Regexp()).OnElements("a")
		sanitizer.policy.AllowAttrs("role").Matching(lazyregexp.New(`^doc-(endnotes|noteref|backlink)$`).Regexp()).OnElements("div", "a")

		for scheme, allow := range urlSchemePolicies {
			sanitizer.policy.AllowURLSchemeWithCustomPolicy(scheme, allow)
		}

		// Custom URL-Schemes
		sanitizer.policy.AllowURLSchemes(conf.Markdown.CustomURLSchemes...)

		// The schemes that bluemonday.UGCPolicy allows, plus the custom ones.
		sanitizer.schemes = map[string]bool{"mailto": true, "http": true, "https": true}
		for _, scheme := range conf.Markdown.CustomURLSchemes {
			sanitizer.schemes[strings.ToLower(scheme)] = true
		}
		sanitizer.ready.Store(true)
	})
}

// IsSafeDataURI returns whether the given data URI uses a safe image MIME type.
func IsSafeDataURI(u *url.URL) bool {
	// The opaque data of a data URI has the form "mediatype;base64,data" or
	// "mediatype,data". We only allow common image MIME types.
	mediatype, _, _ := strings.Cut(u.Opaque, ";")
	mediatype, _, _ = strings.Cut(mediatype, ",")
	switch strings.TrimSpace(strings.ToLower(mediatype)) {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/x-icon":
		return true
	}
	return false
}

// Sanitize takes a string that contains a HTML fragment or document and applies policy whitelist.
func Sanitize(s string) string {
	return sanitizer.policy.Sanitize(s)
}

// SanitizeBytes takes a []byte slice that contains a HTML fragment or document and applies policy whitelist.
func SanitizeBytes(b []byte) []byte {
	return sanitizer.policy.SanitizeBytes(b)
}
