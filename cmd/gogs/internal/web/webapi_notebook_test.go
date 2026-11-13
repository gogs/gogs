package web

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/markup"
)

func TestParseNotebook(t *testing.T) {
	markup.NewSanitizer()
	intPtr := func(v int) *int { return &v }

	t.Run("nbformat 4", func(t *testing.T) {
		nb, err := parseNotebook([]byte(`{
  "metadata": {"language_info": {"name": "python", "file_extension": ".py"}},
  "nbformat": 4,
  "cells": [
    {"cell_type": "markdown", "source": ["# Title\n", "![plot](attachment:plot.png) [data](data.csv)"],
     "attachments": {"plot.png": {"image/png": "iVBO\nRw0K"}}},
    {"cell_type": "code", "execution_count": 3, "source": "print(1)\nfrom IPython.display import *", "outputs": [
      {"output_type": "stream", "name": "stdout", "text": ["1\n"]},
      {"output_type": "execute_result", "execution_count": 3,
       "data": {"text/plain": ["   a\n", "0  1"], "text/html": ["<table><tr><td onclick=\"x()\">1</td></tr></table>", "<script>alert(1)</script>"]}},
      {"output_type": "display_data", "data": {"text/plain": "<Figure>", "image/png": "iVBO\nRw0K\n", "application/json": {"a": 1}}},
      {"output_type": "display_data", "data": {"application/javascript": "alert(1)"}},
      {"output_type": "error", "ename": "ValueError", "evalue": "bad", "traceback": ["\u001b[31mValueError\u001b[0m", "bad"]}
    ]},
    {"cell_type": "raw", "source": "raw text"}
  ]
}`), "/alice/nb/src/master/docs", nil)
		require.NoError(t, err)

		assert.Equal(t, ".py", nb.FileExtension)
		require.Len(t, nb.Cells, 3)

		md := nb.Cells[0]
		assert.Equal(t, "markdown", md.Type)
		assert.Contains(t, md.HTML, "<h1")
		assert.Contains(t, md.HTML, `src="data:image/png;base64,iVBORw0K"`)
		assert.Contains(t, md.HTML, `href="/alice/nb/src/master/docs/data.csv"`)

		code := nb.Cells[1]
		assert.Equal(t, "code", code.Type)
		assert.Equal(t, "print(1)\nfrom IPython.display import *", code.Source)
		assert.Equal(t, intPtr(3), code.ExecutionCount)
		assert.Equal(t, []repoNotebookOutput{
			{Type: "text", Text: "1\n"},
			{Type: "html", ExecutionCount: intPtr(3), HTML: "<table><tr><td>1</td></tr></table>"},
			{Type: "image", Src: "data:image/png;base64,iVBORw0K"},
			{Type: "error", Text: "\u001b[31mValueError\u001b[0m\nbad"},
		}, code.Outputs)

		assert.Equal(t, repoNotebookCell{Type: "raw", Source: "raw text"}, nb.Cells[2])
	})

	t.Run("nbformat 3", func(t *testing.T) {
		nb, err := parseNotebook([]byte(`{
  "nbformat": 3,
  "worksheets": [{"cells": [
    {"cell_type": "heading", "level": 2, "source": ["Results"]},
    {"cell_type": "code", "language": "python", "prompt_number": 7, "input": ["x"], "outputs": [
      {"output_type": "pyout", "prompt_number": 7, "text": ["42"]},
      {"output_type": "display_data", "png": "iVBO", "text": ["<Figure>"]},
      {"output_type": "pyerr", "ename": "NameError", "evalue": "y", "traceback": []}
    ]}
  ]}]
}`), "/alice/nb/src/master", nil)
		require.NoError(t, err)

		assert.Equal(t, ".py", nb.FileExtension)
		require.Len(t, nb.Cells, 2)
		assert.Contains(t, nb.Cells[0].HTML, "<h2")
		assert.Contains(t, nb.Cells[0].HTML, "Results")
		assert.Equal(t, repoNotebookCell{
			Type:           "code",
			Source:         "x",
			ExecutionCount: intPtr(7),
			Outputs: []repoNotebookOutput{
				{Type: "text", ExecutionCount: intPtr(7), Text: "42"},
				{Type: "image", Src: "data:image/png;base64,iVBO"},
				{Type: "error", Text: "NameError: y"},
			},
		}, nb.Cells[1])
	})

	t.Run("not a notebook", func(t *testing.T) {
		_, err := parseNotebook([]byte(`{"cells": "nope"}`), "", nil)
		assert.Error(t, err)
	})
}
