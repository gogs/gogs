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

		{url: "/admin", want: true},
		{url: "/user/repo", want: true},
	}

	for _, test := range tests {
		t.Run(test.url, func(t *testing.T) {
			assert.Equal(t, test.want, IsSameSite(test.url))
		})
	}
}
