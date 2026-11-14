package web

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseCodeQuery(t *testing.T) {
	tests := []struct {
		name string
		q    string
		want codeQuery
	}{
		{name: "keyword only", q: "  func main ", want: codeQuery{Keyword: "func main"}},
		{
			name: "qualifiers anywhere",
			q:    "org:acme  open file path:cmd/ Repo:acme/api PATH:*.go",
			want: codeQuery{Keyword: "open file", Owner: "acme", Repo: "acme/api", Paths: []string{"cmd/", "*.go"}},
		},
		{name: "user is an owner", q: "user:alice x", want: codeQuery{Keyword: "x", Owner: "alice"}},
		{name: "empty qualifier is a keyword", q: "path: x", want: codeQuery{Keyword: "path: x"}},
		{name: "unknown key is a keyword", q: "http://example.com", want: codeQuery{Keyword: "http://example.com"}},
		{name: "quoted phrase", q: `"org:acme" path:a`, want: codeQuery{Keyword: "org:acme", Paths: []string{"a"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, parseCodeQuery(test.q))
		})
	}
}
