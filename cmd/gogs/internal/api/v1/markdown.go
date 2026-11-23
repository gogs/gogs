package v1

import (
	"gogs.io/gogs/internal/markup"
)

// markdownRequest represents the request body for rendering markdown.
type markdownRequest struct {
	Text    string
	Context string
}

func markdown(c *apiContext, markupOpts markup.Options, form markdownRequest) {
	if form.Text == "" {
		_, _ = c.ResponseWriter().Write([]byte(""))
		return
	}

	_, _ = c.ResponseWriter().Write(markup.Markdown(markupOpts, []byte(form.Text), form.Context, nil))
}

func markdownRaw(c *apiContext, markupOpts markup.Options) {
	body, err := c.Request().Body().Bytes()
	if err != nil {
		c.Error(err, "read body")
		return
	}
	_, _ = c.ResponseWriter().Write(markup.SanitizeBytes(markup.RawMarkdown(markupOpts, body, "")))
}
