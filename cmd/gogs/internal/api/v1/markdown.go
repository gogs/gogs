package v1

import (
	"gogs.io/gogs/internal/markup"
)

// markdownRequest represents the request body for rendering markdown.
type markdownRequest struct {
	Text    string
	Context string
}

func markdown(c *apiContext, form markdownRequest) {
	if form.Text == "" {
		_, _ = c.ResponseWriter().Write([]byte(""))
		return
	}

	_, _ = c.ResponseWriter().Write(markup.Markdown([]byte(form.Text), form.Context, nil))
}

func markdownRaw(c *apiContext) {
	body, err := c.Request().Body().Bytes()
	if err != nil {
		c.Error(err, "read body")
		return
	}
	_, _ = c.ResponseWriter().Write(markup.SanitizeBytes(markup.RawMarkdown(body, "")))
}
