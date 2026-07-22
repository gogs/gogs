package repo

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/markup"
)

func TestWikiMarkdownURLPrefix(t *testing.T) {
	content := markup.Markdown("[Home](Home)", wikiMarkdownURLPrefix("/alice/project"), nil)

	assert.Contains(t, string(content), `href="/alice/project/wiki/Home"`)
}
