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

		// Data URIs: safe image types should be allowed
		{input: `<img src="data:image/png;base64,abc">`, expVal: `<img src="data:image/png;base64,abc">`},
		{input: `<img src="data:image/jpeg;base64,abc">`, expVal: `<img src="data:image/jpeg;base64,abc">`},
		{input: `<img src="data:image/gif;base64,abc">`, expVal: `<img src="data:image/gif;base64,abc">`},
		{input: `<img src="data:image/webp;base64,abc">`, expVal: `<img src="data:image/webp;base64,abc">`},

		// Data URIs: text/html must be stripped to prevent XSS (GHSA-xrcr-gmf5-2r8j)
		{input: `<a href="data:text/html;base64,PHNjcmlwdD5hbGVydCgnWFNTJyk8L3NjcmlwdD4=">Click</a>`, expVal: `Click`},
		{input: `<a href="data:text/html,<script>alert(1)</script>">XSS</a>`, expVal: `XSS`},
		{input: `<img src="data:text/html;base64,abc">`, expVal: ``},

		// Data URIs: SVG must be stripped (can contain embedded JavaScript)
		{input: `<img src="data:image/svg+xml;base64,abc">`, expVal: ``},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			assert.Equal(t, test.expVal, Sanitize(test.input))
			assert.Equal(t, test.expVal, string(SanitizeBytes([]byte(test.input))))
		})
	}
}
