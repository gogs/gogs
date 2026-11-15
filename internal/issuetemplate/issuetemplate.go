// Package issuetemplate reads the issue and pull request templates of a
// repository the way GitHub does: Markdown templates and issue forms in an
// ISSUE_TEMPLATE directory, its config.yml, and single template files.
package issuetemplate

import (
	"bytes"
	"context"
	"path"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"gopkg.in/yaml.v3"

	"gogs.io/gogs/internal/git"
)

// locations are the directories templates are read from, in the order they
// are looked at. ".gogs" comes before ".github" so a repository mirrored from
// GitHub can override the templates for Gogs.
var locations = []string{"", ".gogs", ".github", "docs"}

// List is a comma-separated string or a sequence of strings, e.g., the labels
// of a template.
type List []string

func (l *List) UnmarshalYAML(node *yaml.Node) error {
	var values []string
	switch node.Kind {
	case yaml.ScalarNode:
		values = strings.Split(node.Value, ",")
	case yaml.SequenceNode:
		if err := node.Decode(&values); err != nil {
			return err
		}
	default:
		return errors.Newf("line %d: expect a string or a list of strings", node.Line)
	}
	*l = (*l)[:0]
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			*l = append(*l, v)
		}
	}
	return nil
}

// Option is an option of a dropdown or a checkboxes element. The options of a
// dropdown are plain strings.
type Option struct {
	Label    string `yaml:"label"`
	Required bool   `yaml:"required"`
}

func (o *Option) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		o.Label = node.Value
		return nil
	}
	type plain Option
	return node.Decode((*plain)(o))
}

// Element types of an issue form.
const (
	ElementMarkdown   = "markdown"
	ElementTextarea   = "textarea"
	ElementInput      = "input"
	ElementDropdown   = "dropdown"
	ElementCheckboxes = "checkboxes"
)

// Element is an element of the body of an issue form.
type Element struct {
	Type       string `yaml:"type"`
	ID         string `yaml:"id"`
	Attributes struct {
		Label       string   `yaml:"label"`
		Description string   `yaml:"description"`
		Placeholder string   `yaml:"placeholder"`
		Value       string   `yaml:"value"`
		Render      string   `yaml:"render"`
		Multiple    bool     `yaml:"multiple"`
		Options     []Option `yaml:"options"`
		// Default is the index of the option a dropdown starts with.
		Default *int `yaml:"default"`
	} `yaml:"attributes"`
	Validations struct {
		Required bool `yaml:"required"`
	} `yaml:"validations"`
}

// Template is an issue template, either a Markdown template or an issue form.
type Template struct {
	// File is the name of the template file in the ISSUE_TEMPLATE directory.
	File      string
	Name      string
	About     string
	Title     string
	Labels    List
	Assignees List
	// Body is the content of a Markdown template.
	Body string
	// Form is the body of an issue form, nil for a Markdown template.
	Form []*Element
}

// IsForm reports whether the template is an issue form.
func (t *Template) IsForm() bool {
	return t.Form != nil
}

// ContactLink is a link the template chooser offers in place of opening an
// issue.
type ContactLink struct {
	Name  string `yaml:"name"`
	URL   string `yaml:"url"`
	About string `yaml:"about"`
}

// Config is the config.yml of the ISSUE_TEMPLATE directory.
type Config struct {
	BlankIssuesEnabled bool
	ContactLinks       []ContactLink
}

// IssueTemplates are the issue templates of a repository.
type IssueTemplates struct {
	// Templates are sorted by file name.
	Templates []*Template
	Config    Config
	// Default is the content of the single ISSUE_TEMPLATE.md, which a blank
	// issue starts with.
	Default string
}

// Find returns the template in the given file, or nil.
func (ts *IssueTemplates) Find(file string) *Template {
	for _, t := range ts.Templates {
		if t.File == file {
			return t
		}
	}
	return nil
}

// lookup returns the entry with the given name in the tree, or nil. Names
// match case-insensitively like on GitHub, preferring an exact match.
func lookup(ctx context.Context, tree *git.Tree, name string) (*git.TreeEntry, error) {
	entries, err := tree.Entries(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "list tree entries")
	}
	var match *git.TreeEntry
	for _, e := range entries {
		if e.Name() == name {
			return e, nil
		}
		if match == nil && strings.EqualFold(e.Name(), name) {
			match = e
		}
	}
	return match, nil
}

// subtree returns the directory with the given name in the tree, or nil.
func subtree(ctx context.Context, tree *git.Tree, name string) (*git.Tree, error) {
	if name == "" {
		return tree, nil
	}
	e, err := lookup(ctx, tree, name)
	if err != nil || e == nil || !e.IsTree() {
		return nil, err
	}
	sub, err := tree.Subtree(ctx, e.Name())
	if err != nil {
		return nil, errors.Wrapf(err, "get subtree %q", e.Name())
	}
	return sub, nil
}

// readFile returns the content of the file with the given name in the tree.
func readFile(ctx context.Context, tree *git.Tree, name string) (content []byte, found bool, err error) {
	e, err := lookup(ctx, tree, name)
	if err != nil || e == nil || !e.IsBlob() {
		return nil, false, err
	}
	content, err = e.Blob().Bytes(ctx)
	if err != nil {
		return nil, false, errors.Wrapf(err, "read %q", e.Name())
	}
	return content, true, nil
}

// directories returns the existing template locations in the commit.
func directories(ctx context.Context, commit *git.Commit) ([]*git.Tree, error) {
	var dirs []*git.Tree
	for _, loc := range locations {
		dir, err := subtree(ctx, commit.Tree, loc)
		if err != nil {
			return nil, err
		}
		if dir != nil {
			dirs = append(dirs, dir)
		}
	}
	return dirs, nil
}

// FindFile returns the content of the first file found with one of the given
// names in the template locations of the commit.
func FindFile(ctx context.Context, commit *git.Commit, names ...string) (content string, found bool, err error) {
	dirs, err := directories(ctx, commit)
	if err != nil {
		return "", false, err
	}
	for _, dir := range dirs {
		for _, name := range names {
			p, found, err := readFile(ctx, dir, name)
			if err != nil || found {
				return string(p), found, err
			}
		}
	}
	return "", false, nil
}

// PullRequest returns the pull request template of the commit. A non-empty
// name selects a template in a PULL_REQUEST_TEMPLATE directory, like the
// "template" query parameter on GitHub.
func PullRequest(ctx context.Context, commit *git.Commit, name string) (content string, found bool, err error) {
	if name == "" {
		return FindFile(ctx, commit, "PULL_REQUEST_TEMPLATE.md", "PULL_REQUEST.md")
	}
	if strings.ContainsAny(name, `/\`) {
		return "", false, nil
	}
	dirs, err := directories(ctx, commit)
	if err != nil {
		return "", false, err
	}
	for _, dir := range dirs {
		sub, err := subtree(ctx, dir, "PULL_REQUEST_TEMPLATE")
		if err != nil {
			return "", false, err
		}
		if sub == nil {
			continue
		}
		p, found, err := readFile(ctx, sub, name)
		if err != nil || found {
			return string(p), found, err
		}
	}
	return "", false, nil
}

// LoadIssueTemplates returns the issue templates of the commit. Templates that
// are not valid are left out, like on GitHub.
func LoadIssueTemplates(ctx context.Context, commit *git.Commit) (*IssueTemplates, error) {
	ts := &IssueTemplates{Config: Config{BlankIssuesEnabled: true}}
	dirs, err := directories(ctx, commit)
	if err != nil {
		return nil, err
	}

	foundDefault := false
	for _, dir := range dirs {
		if !foundDefault {
			p, found, err := readFile(ctx, dir, "ISSUE_TEMPLATE.md")
			if err != nil {
				return nil, err
			}
			ts.Default, foundDefault = string(p), found
		}
		if ts.Templates != nil {
			continue
		}

		sub, err := subtree(ctx, dir, "ISSUE_TEMPLATE")
		if err != nil {
			return nil, err
		}
		if sub == nil {
			continue
		}
		entries, err := sub.Entries(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "list issue templates")
		}
		ts.Templates = []*Template{}
		for _, e := range entries {
			if !e.IsBlob() {
				continue
			}
			ext := strings.ToLower(path.Ext(e.Name()))
			if ext != ".md" && ext != ".yml" && ext != ".yaml" {
				continue
			}
			p, err := e.Blob().Bytes(ctx)
			if err != nil {
				return nil, errors.Wrapf(err, "read %q", e.Name())
			}
			if strings.EqualFold(strings.TrimSuffix(e.Name(), path.Ext(e.Name())), "config") && ext != ".md" {
				// An invalid config is ignored, like on GitHub.
				if cfg, err := parseConfig(p); err == nil {
					ts.Config = cfg
				}
				continue
			}

			var t *Template
			if ext == ".md" {
				t, err = parseMarkdown(p)
			} else {
				t, err = parseForm(p)
			}
			if err != nil {
				continue
			}
			t.File = e.Name()
			ts.Templates = append(ts.Templates, t)
		}
	}
	return ts, nil
}

func parseConfig(p []byte) (Config, error) {
	var raw struct {
		BlankIssuesEnabled *bool         `yaml:"blank_issues_enabled"`
		ContactLinks       []ContactLink `yaml:"contact_links"`
	}
	if err := yaml.Unmarshal(p, &raw); err != nil {
		return Config{}, err
	}
	cfg := Config{BlankIssuesEnabled: raw.BlankIssuesEnabled == nil || *raw.BlankIssuesEnabled}
	for _, link := range raw.ContactLinks {
		if link.Name == "" || !(strings.HasPrefix(link.URL, "https://") || strings.HasPrefix(link.URL, "http://")) {
			continue
		}
		cfg.ContactLinks = append(cfg.ContactLinks, link)
	}
	return cfg, nil
}

type header struct {
	Name        string `yaml:"name"`
	About       string `yaml:"about"`
	Description string `yaml:"description"`
	Title       string `yaml:"title"`
	Labels      List   `yaml:"labels"`
	Assignees   List   `yaml:"assignees"`
}

// parseMarkdown parses a Markdown template, which needs front matter with a
// name to be offered.
func parseMarkdown(p []byte) (*Template, error) {
	p = bytes.TrimPrefix(p, []byte("\ufeff"))
	normalized := bytes.ReplaceAll(p, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(normalized, []byte("---\n")) {
		return nil, errors.New("missing front matter")
	}
	rest := normalized[len("---\n"):]
	var front, body []byte
	if bytes.HasPrefix(rest, []byte("---\n")) {
		body = rest[len("---\n"):]
	} else {
		end := bytes.Index(rest, []byte("\n---\n"))
		if end < 0 {
			if !bytes.HasSuffix(rest, []byte("\n---")) {
				return nil, errors.New("unterminated front matter")
			}
			end = len(rest) - len("\n---")
			front = rest[:end]
		} else {
			front, body = rest[:end], rest[end+len("\n---\n"):]
		}
	}

	var h header
	if err := yaml.Unmarshal(front, &h); err != nil {
		return nil, errors.Wrap(err, "parse front matter")
	}
	if strings.TrimSpace(h.Name) == "" {
		return nil, errors.New("missing name")
	}
	return &Template{
		Name:      strings.TrimSpace(h.Name),
		About:     strings.TrimSpace(h.About),
		Title:     h.Title,
		Labels:    h.Labels,
		Assignees: h.Assignees,
		Body:      strings.TrimLeft(string(body), "\n"),
	}, nil
}

// parseForm parses and validates an issue form.
func parseForm(p []byte) (*Template, error) {
	var raw struct {
		header `yaml:",inline"`
		Body   []*Element `yaml:"body"`
	}
	if err := yaml.Unmarshal(p, &raw); err != nil {
		return nil, errors.Wrap(err, "parse issue form")
	}
	if strings.TrimSpace(raw.Name) == "" || strings.TrimSpace(raw.Description) == "" {
		return nil, errors.New("missing name or description")
	}

	ids := map[string]bool{}
	hasField := false
	for i, el := range raw.Body {
		if el == nil {
			return nil, errors.Newf("body[%d]: empty element", i)
		}
		if el.ID != "" {
			if ids[el.ID] {
				return nil, errors.Newf("body[%d]: duplicate id %q", i, el.ID)
			}
			ids[el.ID] = true
		}
		switch el.Type {
		case ElementMarkdown:
			if strings.TrimSpace(el.Attributes.Value) == "" {
				return nil, errors.Newf("body[%d]: missing value", i)
			}
			continue
		case ElementTextarea, ElementInput:
		case ElementDropdown, ElementCheckboxes:
			if len(el.Attributes.Options) == 0 {
				return nil, errors.Newf("body[%d]: missing options", i)
			}
			for _, o := range el.Attributes.Options {
				if strings.TrimSpace(o.Label) == "" {
					return nil, errors.Newf("body[%d]: empty option", i)
				}
			}
			if d := el.Attributes.Default; d != nil && (*d < 0 || *d >= len(el.Attributes.Options)) {
				return nil, errors.Newf("body[%d]: default out of range", i)
			}
		default:
			return nil, errors.Newf("body[%d]: unknown type %q", i, el.Type)
		}
		if strings.TrimSpace(el.Attributes.Label) == "" {
			return nil, errors.Newf("body[%d]: missing label", i)
		}
		hasField = true
	}
	if !hasField {
		return nil, errors.New("body has no field")
	}
	return &Template{
		Name:      strings.TrimSpace(raw.Name),
		About:     strings.TrimSpace(raw.Description),
		Title:     raw.Title,
		Labels:    raw.Labels,
		Assignees: raw.Assignees,
		Form:      raw.Body,
	}, nil
}

// FieldError is a value of an issue form field that is not valid.
type FieldError struct {
	// Index is the position of the element in the body of the form.
	Index int
	// Label is the label of the field, or of the checkbox when a required one
	// is not checked.
	Label string
	// Missing is true when a required value is missing, otherwise Message
	// says what is wrong.
	Missing bool
	Message string
}

func (e *FieldError) Error() string {
	if e.Missing {
		return e.Label + " is required"
	}
	return e.Label + ": " + e.Message
}

const noResponse = "_No response_"

// RenderForm returns the issue content for the values of the fields of an
// issue form, in the format GitHub uses. values has an entry per element of
// the form: the text of an input or a textarea, or the chosen option labels of
// a dropdown or checkboxes. It returns a *FieldError when a value is not
// valid.
func (t *Template) RenderForm(values [][]string) (string, error) {
	if len(values) != len(t.Form) {
		return "", errors.Newf("expect %d form values but got %d", len(t.Form), len(values))
	}

	var sections []string
	for i, el := range t.Form {
		if el.Type == ElementMarkdown {
			continue
		}
		label := strings.TrimSpace(el.Attributes.Label)
		fail := func(message string) (string, error) {
			return "", &FieldError{Index: i, Label: label, Message: message}
		}
		missing := func(label string) (string, error) {
			return "", &FieldError{Index: i, Label: label, Missing: true}
		}
		var value string
		switch el.Type {
		case ElementInput, ElementTextarea:
			if len(values[i]) > 1 {
				return fail("expect a single value")
			}
			if len(values[i]) == 1 {
				value = strings.TrimSpace(values[i][0])
			}
			if value == "" {
				if el.Validations.Required {
					return missing(label)
				}
				value = noResponse
			} else if el.Type == ElementTextarea && el.Attributes.Render != "" {
				value = "```" + el.Attributes.Render + "\n" + value + "\n```"
			}

		case ElementDropdown:
			chosen, err := chooseOptions(el, values[i])
			if err != nil {
				return fail(err.Error())
			}
			if len(chosen) > 1 && !el.Attributes.Multiple {
				return fail("expect a single option")
			}
			if len(chosen) == 0 {
				if el.Validations.Required {
					return missing(label)
				}
				value = noResponse
			} else {
				value = strings.Join(chosen, ", ")
			}

		case ElementCheckboxes:
			chosen, err := chooseOptions(el, values[i])
			if err != nil {
				return fail(err.Error())
			}
			lines := make([]string, 0, len(el.Attributes.Options))
			for _, o := range el.Attributes.Options {
				mark := " "
				if slices.Contains(chosen, o.Label) {
					mark = "X"
				} else if o.Required {
					return missing(o.Label)
				}
				lines = append(lines, "- ["+mark+"] "+o.Label)
			}
			value = strings.Join(lines, "\n")
		}
		sections = append(sections, "### "+label+"\n\n"+value)
	}
	return strings.Join(sections, "\n\n"), nil
}

// chooseOptions returns the chosen options in the order of the element.
func chooseOptions(el *Element, values []string) ([]string, error) {
	var chosen []string
	for _, v := range values {
		if !slices.ContainsFunc(el.Attributes.Options, func(o Option) bool { return o.Label == v }) {
			return nil, errors.Newf("%q is not an option", v)
		}
	}
	for _, o := range el.Attributes.Options {
		if slices.Contains(values, o.Label) {
			chosen = append(chosen, o.Label)
		}
	}
	return chosen, nil
}
