package issuetemplate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/git"
)

// commitFiles returns the commit of a new repository with the given files.
func commitFiles(t *testing.T, files map[string]string) *git.Commit {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"add", "-A"},
		{"-c", "user.name=gogs", "-c", "user.email=gogs@example.com", "commit", "-q", "--allow-empty", "-m", "init"},
	} {
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		require.NoError(t, err, string(out))
	}
	repo, err := git.Open(dir)
	require.NoError(t, err)
	commit, err := repo.BranchCommit(context.Background(), "main")
	require.NoError(t, err)
	return commit
}

const bugForm = `name: Bug report
description: Something is broken.
title: "[Bug]: "
labels: [bug, triage]
assignees: alice
body:
  - type: markdown
    attributes:
      value: Thanks for reporting!
  - type: input
    id: version
    attributes:
      label: Version
    validations:
      required: true
  - type: textarea
    attributes:
      label: Logs
      render: shell
  - type: dropdown
    attributes:
      label: Browsers
      multiple: true
      options: [Firefox, Chrome, Safari]
  - type: checkboxes
    attributes:
      label: Terms
      options:
        - label: I searched existing issues
          required: true
        - label: I want to fix it
`

func TestLoadIssueTemplates(t *testing.T) {
	ctx := context.Background()

	t.Run("no templates", func(t *testing.T) {
		ts, err := LoadIssueTemplates(ctx, commitFiles(t, map[string]string{"README.md": "hi"}))
		require.NoError(t, err)
		assert.Empty(t, ts.Templates)
		assert.Empty(t, ts.Default)
		assert.True(t, ts.Config.BlankIssuesEnabled)
	})

	t.Run("directory", func(t *testing.T) {
		ts, err := LoadIssueTemplates(ctx, commitFiles(t, map[string]string{
			".github/issue_template.md":            "Describe the issue.",
			".github/ISSUE_TEMPLATE/feature.md":    "---\nname: Feature request\nabout: Suggest an idea.\nlabels: enhancement, idea\n---\n\n## Idea\r\n",
			".github/ISSUE_TEMPLATE/bug.yaml":      bugForm,
			".github/ISSUE_TEMPLATE/README.md":     "Not a template, it has no front matter.",
			".github/ISSUE_TEMPLATE/broken.yml":    "name: Broken\ndescription: No body.\n",
			".github/ISSUE_TEMPLATE/notes.txt":     "---\nname: Not a template\n---\n",
			".github/ISSUE_TEMPLATE/config.yml":    "blank_issues_enabled: false\ncontact_links:\n  - name: Forum\n    url: https://example.com/forum\n    about: Ask questions here.\n  - name: Unsafe\n    url: javascript:alert(1)\n",
			".github/ISSUE_TEMPLATE/nested/c.md":   "---\nname: Nested\n---\n",
			"docs/ISSUE_TEMPLATE/ignored.md":       "---\nname: Not read from docs\n---\n",
			".github/PULL_REQUEST_TEMPLATE/one.md": "One",
		}))
		require.NoError(t, err)

		assert.Equal(t, "Describe the issue.", ts.Default)
		assert.Equal(t, Config{
			BlankIssuesEnabled: false,
			ContactLinks:       []ContactLink{{Name: "Forum", URL: "https://example.com/forum", About: "Ask questions here."}},
		}, ts.Config)

		require.Len(t, ts.Templates, 2)
		bug, feature := ts.Templates[0], ts.Templates[1]
		assert.Equal(t, "bug.yaml", bug.File)
		assert.True(t, bug.IsForm())
		assert.Equal(t, "Something is broken.", bug.About)
		assert.Equal(t, "[Bug]: ", bug.Title)
		assert.Equal(t, List{"bug", "triage"}, bug.Labels)
		assert.Equal(t, List{"alice"}, bug.Assignees)
		assert.Len(t, bug.Form, 5)

		assert.Equal(t, "feature.md", feature.File)
		assert.False(t, feature.IsForm())
		assert.Equal(t, "Feature request", feature.Name)
		assert.Equal(t, "Suggest an idea.", feature.About)
		assert.Equal(t, List{"enhancement", "idea"}, feature.Labels)
		assert.Equal(t, "## Idea\n", feature.Body)

		assert.Same(t, feature, ts.Find("feature.md"))
		assert.Nil(t, ts.Find("README.md"))
	})

	t.Run(".gogs takes precedence over .github", func(t *testing.T) {
		ts, err := LoadIssueTemplates(ctx, commitFiles(t, map[string]string{
			".gogs/ISSUE_TEMPLATE/a.md":   "---\nname: Gogs\n---\n",
			".github/ISSUE_TEMPLATE/b.md": "---\nname: GitHub\n---\n",
			".github/ISSUE_TEMPLATE.md":   "GitHub default",
		}))
		require.NoError(t, err)
		require.Len(t, ts.Templates, 1)
		assert.Equal(t, "Gogs", ts.Templates[0].Name)
		assert.Equal(t, "GitHub default", ts.Default)
	})
}

func TestPullRequest(t *testing.T) {
	ctx := context.Background()
	commit := commitFiles(t, map[string]string{
		".github/pull_request_template.md":         "Default",
		"docs/PULL_REQUEST_TEMPLATE/docs.md":       "Not read from docs",
		".github/PULL_REQUEST_TEMPLATE/feature.md": "Feature",
		"PULL_REQUEST_TITLE.md":                    "{{headBranch}}",
	})

	tests := []struct {
		name        string
		template    string
		wantContent string
		wantFound   bool
	}{
		{name: "default", template: "", wantContent: "Default", wantFound: true},
		{name: "named", template: "feature.md", wantContent: "Feature", wantFound: true},
		{name: "missing", template: "nope.md", wantFound: false},
		{name: "docs is not a location", template: "docs.md", wantFound: false},
		{name: "path traversal", template: "../pull_request_template.md", wantFound: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			content, found, err := PullRequest(ctx, commit, test.template)
			require.NoError(t, err)
			assert.Equal(t, test.wantFound, found)
			assert.Equal(t, test.wantContent, content)
		})
	}

	content, found, err := FindFile(ctx, commit, "PULL_REQUEST_TITLE.md")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "{{headBranch}}", content)

	t.Run("legacy name", func(t *testing.T) {
		content, found, err := PullRequest(ctx, commitFiles(t, map[string]string{".gogs/PULL_REQUEST.md": "Legacy"}), "")
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, "Legacy", content)
	})
}

func TestTemplate_RenderForm(t *testing.T) {
	tmpl, err := parseForm([]byte(bugForm))
	require.NoError(t, err)

	t.Run("valid", func(t *testing.T) {
		got, err := tmpl.RenderForm([][]string{
			nil,
			{" 1.2.3 "},
			{"panic: oops"},
			{"Safari", "Firefox"},
			{"I searched existing issues"},
		}, nil)
		require.NoError(t, err)
		want := "### Version\n\n1.2.3\n\n" +
			"### Logs\n\n```shell\npanic: oops\n```\n\n" +
			"### Browsers\n\nFirefox, Safari\n\n" +
			"### Terms\n\n- [X] I searched existing issues\n- [ ] I want to fix it"
		assert.Equal(t, want, got)
	})

	t.Run("no response", func(t *testing.T) {
		got, err := tmpl.RenderForm([][]string{nil, {"1"}, {"  "}, nil, {"I searched existing issues"}}, nil)
		require.NoError(t, err)
		assert.Contains(t, got, "### Logs\n\n_No response_\n\n### Browsers\n\n_No response_\n\n")
	})

	tests := []struct {
		name        string
		values      [][]string
		wantIndexes []int
	}{
		{name: "missing required input", values: [][]string{nil, {" "}, nil, nil, {"I searched existing issues"}}, wantIndexes: []int{1}},
		{name: "unknown option", values: [][]string{nil, {"1"}, nil, {"Edge"}, {"I searched existing issues"}}, wantIndexes: []int{3}},
		{name: "missing required checkbox", values: [][]string{nil, {"1"}, nil, nil, {"I want to fix it"}}, wantIndexes: []int{4}},
		{name: "every invalid value", values: [][]string{nil, nil, nil, {"Edge"}, nil}, wantIndexes: []int{1, 3, 4}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := tmpl.RenderForm(test.values, nil)
			var fieldErrs FieldErrors
			require.ErrorAs(t, err, &fieldErrs)
			var indexes []int
			for _, fieldErr := range fieldErrs {
				indexes = append(indexes, fieldErr.Index)
			}
			assert.Equal(t, test.wantIndexes, indexes)
		})
	}

	_, err = tmpl.RenderForm([][]string{nil}, nil)
	assert.Error(t, err)
}

func TestParseMarkdown_FrontMatterDelimiters(t *testing.T) {
	tmpl, err := parseMarkdown([]byte("--- \r\nname: A\n---\t\nBody\n"))
	require.NoError(t, err)
	assert.Equal(t, "A", tmpl.Name)
	assert.Equal(t, "Body\n", tmpl.Body)

	_, err = parseMarkdown([]byte("---\nname: A\n---- \nBody\n"))
	assert.Error(t, err)
}

func TestTemplate_RenderForm_RequiredCheckboxes(t *testing.T) {
	tmpl, err := parseForm([]byte(`name: A
description: B
body:
  - type: checkboxes
    attributes:
      label: OS
      options: [mac, linux]
    validations:
      required: true
`))
	require.NoError(t, err)
	_, err = tmpl.RenderForm([][]string{nil}, nil)
	var fieldErrs FieldErrors
	require.ErrorAs(t, err, &fieldErrs)
	assert.True(t, fieldErrs[0].Missing)

	got, err := tmpl.RenderForm([][]string{{"linux"}}, nil)
	require.NoError(t, err)
	assert.Equal(t, "### OS\n\n- [ ] mac\n- [X] linux", got)
}

func TestTemplate_RenderForm_Upload(t *testing.T) {
	tmpl, err := parseForm([]byte(`name: A
description: B
body:
  - type: upload
    attributes:
      label: Screenshots
    validations:
      required: true
      accept: ".png, .tar.gz"
`))
	require.NoError(t, err)
	files := func(id string) (File, bool) {
		f, ok := map[string]File{
			"a": {Name: "shot [1].PNG", URL: "https://example.com/attachments/a", IsImage: true},
			"b": {Name: "logs.tar.gz", URL: "https://example.com/attachments/b"},
			"c": {Name: "notes.txt", URL: "https://example.com/attachments/c"},
		}[id]
		return f, ok
	}

	got, err := tmpl.RenderForm([][]string{{"a", "b"}}, files)
	require.NoError(t, err)
	assert.Equal(t, "### Screenshots\n\n![shot \\[1\\].PNG](https://example.com/attachments/a)\n[logs.tar.gz](https://example.com/attachments/b)", got)

	for _, values := range [][]string{nil, {"c"}, {"missing"}} {
		_, err = tmpl.RenderForm([][]string{values}, files)
		var fieldErrs FieldErrors
		require.ErrorAs(t, err, &fieldErrs, values)
	}

	// Without uploads, the field cannot be filled in, so it is not required.
	got, err = tmpl.RenderForm([][]string{nil}, nil)
	require.NoError(t, err)
	assert.Equal(t, "### Screenshots\n\n_No response_", got)
}

func TestParseForm_SingleDropdown(t *testing.T) {
	tmpl, err := parseForm([]byte(`name: A
description: B
body:
  - type: dropdown
    attributes:
      label: Pick
      options: [x, y]
`))
	require.NoError(t, err)
	_, err = tmpl.RenderForm([][]string{{"x", "y"}}, nil)
	var fieldErrs FieldErrors
	require.ErrorAs(t, err, &fieldErrs)
}
