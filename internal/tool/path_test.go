package tool

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_IsSameSiteURLPath(t *testing.T) {
	tests := []struct {
		url    string
		expVal bool
	}{
		{url: "//github.com", expVal: false},
		{url: "http://github.com", expVal: false},
		{url: "https://github.com", expVal: false},
		{url: "/\\github.com", expVal: false},

		{url: "/admin", expVal: true},
		{url: "/user/repo", expVal: true},
	}

	for _, test := range tests {
		t.Run(test.url, func(t *testing.T) {
			assert.Equal(t, test.expVal, IsSameSiteURLPath(test.url))
		})
	}
}
