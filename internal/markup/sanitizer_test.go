// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package markup_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	. "gogs.io/gogs/internal/markup"
)

func Test_Sanitizer(t *testing.T) {
	NewSanitizer()
	tests := []struct {
		input  string
		expVal string
	}{
		// Regular
		{input: `<a onblur="alert(secret)" href="http://www.google.com">Google</a>`, expVal: `<a href="http://www.google.com" rel="nofollow">Google</a>`},

		// Code highlighting class
		{input: `<code class="random string"></code>`, expVal: `<code></code>`},
		{input: `<code class="language-random ui tab active menu attached animating sidebar following bar center"></code>`, expVal: `<code></code>`},
		{input: `<code class="language-go"></code>`, expVal: `<code class="language-go"></code>`},

		// Input checkbox
		{input: `<input type="hidden">`, expVal: ``},
		{input: `<input type="checkbox">`, expVal: `<input type="checkbox">`},
		{input: `<input checked disabled autofocus>`, expVal: `<input checked="" disabled="">`},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			assert.Equal(t, test.expVal, Sanitize(test.input))
			assert.Equal(t, test.expVal, string(SanitizeBytes([]byte(test.input))))
		})
	}
}
