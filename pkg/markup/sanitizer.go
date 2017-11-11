// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package markup

import (
	"regexp"
	"sync"

	"github.com/microcosm-cc/bluemonday"

	"github.com/gogits/gogs/pkg/setting"
)

// Sanitizer is a protection wrapper of *bluemonday.Policy which does not allow
// any modification to the underlying policies once it's been created.
type Sanitizer struct {
	policy *bluemonday.Policy
	init   sync.Once
}

var sanitizer = &Sanitizer{
	policy: bluemonday.UGCPolicy(),
}

// NewSanitizer initializes sanitizer with allowed attributes based on settings.
// Multiple calls to this function will only create one instance of Sanitizer during
// entire application lifecycle.
func NewSanitizer() {
	sanitizer.init.Do(func() {
		// We only want to allow HighlightJS specific classes for code blocks
		sanitizer.policy.AllowAttrs("class").Matching(regexp.MustCompile(`^language-\w+$`)).OnElements("code")

		// Checkboxes
		sanitizer.policy.AllowAttrs("type").Matching(regexp.MustCompile(`^checkbox$`)).OnElements("input")
		sanitizer.policy.AllowAttrs("checked", "disabled").OnElements("input")

		// Custom URL-Schemes
		sanitizer.policy.AllowURLSchemes(setting.Markdown.CustomURLSchemes...)
	})
}

// Sanitize takes a string that contains a HTML fragment or document and applies policy whitelist.
func Sanitize(s string) string {
	return sanitizer.policy.Sanitize(s)
}

// SanitizeBytes takes a []byte slice that contains a HTML fragment or document and applies policy whitelist.
func SanitizeBytes(b []byte) []byte {
	return sanitizer.policy.SanitizeBytes(b)
}
