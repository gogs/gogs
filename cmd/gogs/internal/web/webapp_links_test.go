package web

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/markup"
)

func TestMarkAppLinks(t *testing.T) {
	markup.NewSanitizer()
	conf.SetMockServer(t, conf.ServerOpts{ExternalURL: "https://example.com/gogs/", Subpath: "/gogs", SubpathDepth: 1})
	repo := &database.Repository{
		Name:               "demo",
		Owner:              &database.User{Name: "alice"},
		EnableIssues:       true,
		EnableWiki:         true,
		EnableExternalWiki: true,
		ExternalWikiURL:    "https://wiki.example.com",
	}

	tests := []struct {
		href string
		want bool
	}{
		{href: "/gogs/alice/demo", want: true},
		{href: "/gogs/alice/demo/src/main/README.md", want: true},
		{href: "/gogs/alice/demo/commit/0123abc#diff-1", want: true},
		{href: "https://example.com/gogs/bob/other/pulls/2/files", want: true},
		{href: "/gogs/alice", want: true},
		{href: "/gogs/alice/demo/issues", want: true},
		{href: "/gogs/bob/other/issues/7", want: true},
		{href: "/gogs/alice/demo/src/main/../../raw/main/a.png", want: false},
		{href: "/gogs/../gogs/attachments/1", want: false},
		{href: "/gogs/../other/app", want: false},

		{href: "/alice/demo", want: false},
		{href: "/gogsx/alice/demo", want: false},
		{href: "//example.com/gogs/alice/demo", want: false},
		{href: "https://other.example.com/gogs/alice/demo", want: false},
		{href: "#heading", want: false},
		{href: "/gogs/attachments/0b2f5d1e", want: false},
		{href: "/gogs/-/avatars/1", want: false},
		{href: "/gogs/api/v1/repos/alice/demo", want: false},
		{href: "/gogs/img/favicon.png", want: false},
		{href: "/gogs/alice.keys", want: false},
		{href: "/gogs/explore", want: false},
		{href: "/gogs/alice/demo?go-get=1", want: false},
		{href: "/gogs/alice/demo/raw/main/a.png", want: false},
		{href: "/gogs/alice/demo/archive/main.zip", want: false},
		{href: "/gogs/alice/demo/commit/0123abc.patch", want: false},
		{href: "/gogs/alice/demo/compare/main...dev.diff", want: false},
		{href: "/gogs/alice/demo/pulls/2.diff", want: false},
		// The server may send the issues and the wiki of other repositories to
		// their external tracker and wiki, and this one has an external wiki.
		{href: "/gogs/bob/other/issues", want: false},
		{href: "/gogs/alice/demo/wiki/Home", want: false},
		{href: "/gogs/bob/other/wiki", want: false},
	}
	for _, test := range tests {
		t.Run(test.href, func(t *testing.T) {
			got := markAppLinks(markup.Markdown(fmt.Sprintf(`<a href="%s">link</a>`, test.href), repo.Link(), nil), repo)
			assert.Equal(t, test.want, strings.Contains(got, "data-app-link"), got)
		})
	}

	t.Run("raw HTML", func(t *testing.T) {
		got := markAppLinks(markup.Markdown(`<a title="Issue" href="/gogs/alice/demo/issues/1">#1</a> <a data-app-link href="/gogs/attachments/1">file</a> @bob`, repo.Link(), nil), repo)
		assert.Equal(t, `<p><a data-app-link title="Issue" href="/gogs/alice/demo/issues/1" rel="nofollow">#1</a> <a href="/gogs/attachments/1" rel="nofollow">file</a> <a data-app-link href="/gogs/bob" rel="nofollow">@bob</a></p>`+"\n", got)
	})
}
