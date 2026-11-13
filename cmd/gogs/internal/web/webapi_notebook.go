package web

import (
	"encoding/json"
	"strings"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/markup"
)

// repoNotebook is a Jupyter notebook to show, with its Markdown rendered and
// its HTML sanitized like any other markup of a repository.
type repoNotebook struct {
	// FileExtension is the extension of the source files of the notebook's
	// language with its leading dot, e.g., ".py", or empty when not known.
	FileExtension string             `json:"fileExtension"`
	Cells         []repoNotebookCell `json:"cells"`
}

type repoNotebookCell struct {
	// Type is "markdown", "code" or "raw".
	Type string `json:"type"`
	// HTML is set for a Markdown cell, and Source otherwise.
	HTML           string               `json:"html,omitempty"`
	Source         string               `json:"source,omitempty"`
	ExecutionCount *int                 `json:"executionCount,omitempty"`
	Outputs        []repoNotebookOutput `json:"outputs,omitempty"`
}

type repoNotebookOutput struct {
	// Type is "text", "error", "image" or "html". The text of "text" and
	// "error" may contain ANSI escape sequences.
	Type string `json:"type"`
	// ExecutionCount is set for the result of a cell.
	ExecutionCount *int   `json:"executionCount,omitempty"`
	Text           string `json:"text,omitempty"`
	// Src is the data URI of an image.
	Src  string `json:"src,omitempty"`
	HTML string `json:"html,omitempty"`
}

// ipynbText is text that notebooks store either as one string or as a list
// of lines.
type ipynbText string

func (t *ipynbText) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*t = ipynbText(s)
		return nil
	}
	var lines []string
	if err := json.Unmarshal(b, &lines); err != nil {
		return err
	}
	*t = ipynbText(strings.Join(lines, ""))
	return nil
}

type ipynbOutput struct {
	OutputType     string `json:"output_type"`
	ExecutionCount *int   `json:"execution_count"`
	// Data maps MIME types to values, most of which are text.
	Data      map[string]json.RawMessage `json:"data"`
	Text      ipynbText                  `json:"text"`
	Traceback []string                   `json:"traceback"`
	EName     string                     `json:"ename"`
	EValue    string                     `json:"evalue"`

	// Fields of nbformat 3, which keeps the data of a result next to its text.
	PromptNumber *int      `json:"prompt_number"`
	HTML         ipynbText `json:"html"`
	PNG          ipynbText `json:"png"`
	JPEG         ipynbText `json:"jpeg"`
}

type ipynbCell struct {
	CellType       string                          `json:"cell_type"`
	Source         ipynbText                       `json:"source"`
	ExecutionCount *int                            `json:"execution_count"`
	Outputs        []ipynbOutput                   `json:"outputs"`
	Attachments    map[string]map[string]ipynbText `json:"attachments"`

	// Fields of nbformat 3, which has heading cells and calls the source of a
	// code cell its input.
	Input        ipynbText `json:"input"`
	PromptNumber *int      `json:"prompt_number"`
	Level        int       `json:"level"`
}

type ipynbNotebook struct {
	Cells    []ipynbCell `json:"cells"`
	Metadata struct {
		LanguageInfo struct {
			FileExtension string `json:"file_extension"`
		} `json:"language_info"`
	} `json:"metadata"`
	// Worksheets hold the cells in nbformat 3.
	Worksheets []struct {
		Cells []ipynbCell `json:"cells"`
	} `json:"worksheets"`
}

// parseNotebook reads a Jupyter notebook in nbformat 3 or 4. Relative links
// in its Markdown resolve against dirLink.
func parseNotebook(p []byte, dirLink string, metas map[string]string) (*repoNotebook, error) {
	var raw ipynbNotebook
	if err := json.Unmarshal(p, &raw); err != nil {
		return nil, errors.Wrap(err, "decode notebook")
	}
	cells := raw.Cells
	for _, ws := range raw.Worksheets {
		cells = append(cells, ws.Cells...)
	}

	nb := &repoNotebook{
		FileExtension: raw.Metadata.LanguageInfo.FileExtension,
		Cells:         make([]repoNotebookCell, 0, len(cells)),
	}
	if nb.FileExtension == "" && len(raw.Worksheets) > 0 {
		// Notebooks of nbformat 3 are all from IPython.
		nb.FileExtension = ".py"
	}
	for _, c := range cells {
		switch c.CellType {
		case "markdown", "heading":
			source := string(c.Source)
			if c.CellType == "heading" {
				source = strings.Repeat("#", max(c.Level, 1)) + " " + source
			}
			source = inlineAttachments(source, c.Attachments)
			nb.Cells = append(nb.Cells, repoNotebookCell{
				Type: "markdown",
				HTML: string(markup.Markdown(source, dirLink, metas)),
			})
		case "code":
			source := c.Source
			if source == "" {
				source = c.Input
			}
			cell := repoNotebookCell{
				Type:           "code",
				Source:         string(source),
				ExecutionCount: firstNonNil(c.ExecutionCount, c.PromptNumber),
			}
			for _, o := range c.Outputs {
				if out, ok := convertNotebookOutput(o, dirLink, metas); ok {
					cell.Outputs = append(cell.Outputs, out)
				}
			}
			nb.Cells = append(nb.Cells, cell)
		case "raw":
			nb.Cells = append(nb.Cells, repoNotebookCell{Type: "raw", Source: string(c.Source)})
		}
	}
	return nb, nil
}

func firstNonNil(values ...*int) *int {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}

// inlineAttachments replaces the "attachment:" links of a Markdown cell with
// data URIs of the images attached to it.
func inlineAttachments(source string, attachments map[string]map[string]ipynbText) string {
	for name, data := range attachments {
		for mime, value := range data {
			if !strings.HasPrefix(mime, "image/") {
				continue
			}
			uri := "data:" + mime + ";base64," + strings.Join(strings.Fields(string(value)), "")
			source = strings.ReplaceAll(source, "attachment:"+name, uri)
			break
		}
	}
	return source
}

// convertNotebookOutput picks the richest kind of an output that a browser
// shows safely, in the order of Jupyter. Outputs without such a kind, e.g.,
// only JavaScript, are left out.
func convertNotebookOutput(o ipynbOutput, dirLink string, metas map[string]string) (repoNotebookOutput, bool) {
	switch o.OutputType {
	case "stream":
		return repoNotebookOutput{Type: "text", Text: string(o.Text)}, true
	case "error", "pyerr":
		text := strings.Join(o.Traceback, "\n")
		if text == "" {
			text = o.EName + ": " + o.EValue
		}
		return repoNotebookOutput{Type: "error", Text: text}, true
	case "execute_result", "display_data", "pyout":
	default:
		return repoNotebookOutput{}, false
	}

	data := make(map[string]string, len(o.Data))
	for mime, value := range o.Data {
		var text ipynbText
		if json.Unmarshal(value, &text) == nil {
			data[mime] = string(text)
		}
	}
	for mime, value := range map[string]ipynbText{"text/html": o.HTML, "image/png": o.PNG, "image/jpeg": o.JPEG, "text/plain": o.Text} {
		if value != "" && data[mime] == "" {
			data[mime] = string(value)
		}
	}

	out := repoNotebookOutput{ExecutionCount: firstNonNil(o.ExecutionCount, o.PromptNumber)}
	switch {
	case data["text/html"] != "":
		out.Type = "html"
		out.HTML = markup.Sanitize(data["text/html"])
	case data["text/markdown"] != "":
		out.Type = "html"
		out.HTML = string(markup.Markdown(data["text/markdown"], dirLink, metas))
	case data["image/png"] != "", data["image/jpeg"] != "", data["image/gif"] != "":
		out.Type = "image"
		for _, mime := range []string{"image/png", "image/jpeg", "image/gif"} {
			if data[mime] != "" {
				out.Src = "data:" + mime + ";base64," + strings.Join(strings.Fields(data[mime]), "")
				break
			}
		}
	case data["text/plain"] != "":
		out.Type = "text"
		out.Text = data["text/plain"]
	default:
		return repoNotebookOutput{}, false
	}
	return out, true
}
