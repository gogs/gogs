package markup

import (
	"bytes"
	"strings"
	"sync"
	"sync/atomic"

	"gogs.io/gogs/internal/markup/dariomark"
)

// Sanitizer sanitizes HTML with an allowlist policy for user content. Its
// policy cannot be modified once it's been created.
type Sanitizer struct {
	init   sync.Once
	ready  atomic.Bool
	policy atomic.Pointer[dariomark.Policy]
	// markdown is the policy as Dariomark uses it while rendering, set
	// before ready.
	markdown *dariomark.Sanitizer
}

var sanitizer = func() *Sanitizer {
	s := &Sanitizer{}
	s.policy.Store(dariomark.NewPolicy(false, nil))
	return s
}()

// NewSanitizer extends the policy of the sanitizer with the rules for rendered
// Markdown and the custom URL schemes. Multiple calls to this function will
// only extend it once during entire application lifecycle.
func NewSanitizer(customURLSchemes []string) {
	sanitizer.init.Do(func() {
		p := dariomark.NewPolicy(true, customURLSchemes)
		sanitizer.policy.Store(p)
		sanitizer.markdown = p.Sanitizer()
		sanitizer.ready.Store(true)
	})
}

// Sanitize takes a string that contains an HTML fragment or document and applies policy allowlist.
func Sanitize(s string) string {
	if strings.TrimSpace(s) == "" {
		return s
	}
	return string(sanitizer.policy.Load().AppendSanitized(make([]byte, 0, len(s)), []byte(s)))
}

// SanitizeBytes takes a []byte slice that contains an HTML fragment or document and applies policy allowlist.
func SanitizeBytes(b []byte) []byte {
	if len(bytes.TrimSpace(b)) == 0 {
		return b
	}
	out := sanitizer.policy.Load().AppendSanitized(make([]byte, 0, len(b)), b)
	if len(out) == 0 {
		return nil
	}
	return out
}
