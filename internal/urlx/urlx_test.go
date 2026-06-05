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
		{url: "/\\github.com", want: false},
		{url: "/a/../\\example.com", want: false},
		{url: "/\\\\example.com", want: false},
		{url: "/%5Cexample.com", want: false},
		{url: "/a/..%5Cexample.com", want: false},
		{url: "/a/../%5Cexample.com", want: false},
		{url: "", want: false},
		{url: "/", want: false},
		{url: "javascript:alert(1)", want: false},

		{url: "/admin", want: true},
		{url: "/user/repo", want: true},
		{url: "/user/repo?key=value", want: true},
		{url: "/user/repo#anchor", want: true},
	}

	for _, test := range tests {
		t.Run(test.url, func(t *testing.T) {
			assert.Equal(t, test.want, IsSameSite(test.url))
		})
	}
}
