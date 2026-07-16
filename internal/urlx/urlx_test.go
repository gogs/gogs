package urlx

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsSameSite(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{url: "//github.com", want: false},
		{url: "http://github.com", want: false},
		{url: "https://github.com", want: false},
		{url: "", want: false},
		{url: "javascript:alert(1)", want: false},

		{url: "/", want: true},
		{url: "/admin", want: true},
		{url: "/user/repo", want: true},
		{url: "/user/repo?key=value", want: true},
		{url: "/user/repo#anchor", want: true},

		// Backslash bypasses: browsers normalize `\` to `/` on Location headers,
		// so any backslash makes the URL not same-site.
		{url: "/\\github.com", want: false},
		{url: "/a/../\\example.com", want: false},
		{url: "/\\\\example.com", want: false},
		{url: "/path\\segment", want: false},
		// Percent-encoded backslashes survive c.Query and reach the browser raw.
		{url: "/%5Cexample.com", want: false},
		{url: "/%5cexample.com", want: false},
		{url: "/a/../%5Cexample.com", want: false},

		// Whitespace bypasses: browsers strip tab/CR/LF before resolving, so any
		// occurrence makes the URL not same-site.
		{url: "/\t/example.com", want: false},
		{url: "/\n/example.com", want: false},
		{url: "/\r/example.com", want: false},
		{url: "/\texample.com", want: false},
	}

	for _, test := range tests {
		t.Run(test.url, func(t *testing.T) {
			assert.Equal(t, test.want, IsSameSite(test.url))
		})
	}
}
