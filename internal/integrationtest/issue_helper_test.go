package integrationtest

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// issueAPIPath returns the path of the web API of the repository followed by
// the elements, e.g., issueAPIPath(repo, "issues", 1).
func issueAPIPath(repo *Repository, elems ...any) string {
	return "/api/web/" + issuePagePath(repo, elems...)[1:]
}

// issuePagePath returns the path of the page of the repository followed by the
// elements, e.g., issuePagePath(repo, "issues", 1).
func issuePagePath(repo *Repository, elems ...any) string {
	var b strings.Builder
	b.WriteString("/" + repo.Owner + "/" + repo.Name)
	for _, e := range elems {
		b.WriteString("/")
		switch v := e.(type) {
		case string:
			b.WriteString(v)
		case int64:
			b.WriteString(strconv.FormatInt(v, 10))
		default:
			panic("unsupported path element")
		}
	}
	return b.String()
}

// issueCreate opens an issue in the repository as the client and returns its
// index.
func issueCreate(t *testing.T, c *client, repo *Repository, title, content string) int64 {
	t.Helper()

	var created struct {
		Index int64 `json:"index"`
	}
	c.doJSON(http.MethodPost, issueAPIPath(repo, "issues"), map[string]any{
		"title":   title,
		"content": content,
	}, http.StatusCreated, &created)
	require.Positive(t, created.Index)
	return created.Index
}

// issuePNG returns a small PNG image, a file type attachments allow by default.
func issuePNG(t *testing.T) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(1, 1, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

// issueUpload uploads the file as the "file" form field to the path, and
// returns the status code and the response body.
func issueUpload(t *testing.T, c *client, path, filename string, content []byte) (int, []byte) {
	t.Helper()

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	status, _, respBody := c.doRaw(http.MethodPost, path, w.FormDataContentType(), &body)
	return status, respBody
}
