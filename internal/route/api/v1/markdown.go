package v1

import (
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/markup"
)

// markdownRequest represents the request body for rendering markdown.
type markdownRequest struct {
	Text    string
	Context string
}

func markdown(c *context.APIContext, form markdownRequest) {
	if form.Text == "" {
		_, _ = c.Write([]byte(""))
		return
	}

	_, _ = c.Write(markup.Markdown([]byte(form.Text), form.Context, nil))
}

func markdownRaw(c *context.APIContext) {
	body, err := c.Req.Body().Bytes()
	if err != nil {
		c.Error(err, "read body")
		return
	}
	_, _ = c.Write(markup.SanitizeBytes(markup.RawMarkdown(body, "")))
}
