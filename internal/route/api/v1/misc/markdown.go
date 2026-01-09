package misc

import (
	api "github.com/gogs/go-gogs-client"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/markup"
)

func Markdown(c *context.APIContext, form api.MarkdownOption) {
	if form.Text == "" {
		_, _ = c.Write([]byte(""))
		return
	}

	_, _ = c.Write(markup.Markdown([]byte(form.Text), form.Context, nil))
}

func MarkdownRaw(c *context.APIContext) {
	body, err := c.Req.Body().Bytes()
	if err != nil {
		c.Error(err, "read body")
		return
	}
	_, _ = c.Write(markup.SanitizeBytes(markup.RawMarkdown(body, "")))
}
