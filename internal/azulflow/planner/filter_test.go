package planner

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/azulflow/model"
)

func TestMatchPatterns(t *testing.T) {
	// Cases from GitHub's filter pattern cheat sheet.
	tests := []struct {
		patterns []string
		name     string
		want     bool
	}{
		{[]string{"feature/*"}, "feature/my-branch", true},
		{[]string{"feature/*"}, "feature/your/branch", false},
		{[]string{"feature/**"}, "feature/your/branch", true},
		{[]string{"main", "releases/mona-the-octocat"}, "releases/mona-the-octocat", true},
		{[]string{"*"}, "releases/v1", false},
		{[]string{"**"}, "releases/v1", true},
		{[]string{"v2*"}, "v2.0", true},
		{[]string{"v[12].[0-9]+.[0-9]+"}, "v1.10.1", true},
		{[]string{"v[12].[0-9]+.[0-9]+"}, "v3.0.0", false},
		{[]string{"v[12].[0-9]+.[0-9]+"}, "v1.x.0", false},
		{[]string{"*.jsx?"}, "page.js", true},
		{[]string{"*.jsx?"}, "page.jsx", true},
		{[]string{"*.jsx?"}, "src/page.js", false},
		{[]string{"**.js"}, "src/js/index.js", true},
		{[]string{"**/README.md"}, "README.md", true},
		{[]string{"**/README.md"}, "server/README.md", true},
		{[]string{"**/*src/**"}, "a/src/app.js", true},
		{[]string{"**/*-post.md"}, "my-post.md", true},
		{[]string{"docs/**"}, "docs/a/b.md", true},
		{[]string{"docs/**"}, "src/docs/b.md", false},
		{[]string{"a\\*b"}, "a*b", true},
		{[]string{"a\\*b"}, "axb", false},
		// The last matching pattern decides.
		{[]string{"releases/**", "!releases/**-alpha"}, "releases/v1-alpha", false},
		{[]string{"releases/**", "!releases/**-alpha"}, "releases/v1", true},
		{[]string{"!releases/**-alpha", "releases/**"}, "releases/v1-alpha", true},
	}
	for _, test := range tests {
		got, err := matchPatterns(test.patterns, test.name)
		require.NoError(t, err)
		assert.Equal(t, test.want, got, "%v matching %q", test.patterns, test.name)
	}

	_, err := matchPatterns([]string{"[abc"}, "a")
	assert.Error(t, err)
}

func TestMatch(t *testing.T) {
	parse := func(on string) *model.Workflow {
		w, err := load("on:\n"+on+"\njobs:\n  a:\n    runs-on: x\n    steps: [{run: x}]\n", nil)
		require.NoError(t, err)
		require.Empty(t, w.Errors)
		return w
	}
	tests := []struct {
		name string
		on   string
		ev   Event
		want bool
	}{
		{"push without filters", "  push:", Event{Name: "push", Ref: "refs/heads/x"}, true},
		{"other event", "  push:", Event{Name: "pull_request", Ref: "refs/heads/x", Action: "opened"}, false},
		{"branch filter", "  push:\n    branches: [main]", Event{Name: "push", Ref: "refs/heads/main"}, true},
		{"branch filter misses", "  push:\n    branches: [main]", Event{Name: "push", Ref: "refs/heads/dev"}, false},
		{"branches only skips tags", "  push:\n    branches: [main]", Event{Name: "push", Ref: "refs/tags/v1"}, false},
		{"tags only skips branches", "  push:\n    tags: [v*]", Event{Name: "push", Ref: "refs/heads/main"}, false},
		{"tag filter", "  push:\n    tags: [v*]", Event{Name: "push", Ref: "refs/tags/v1"}, true},
		{"branches-ignore", "  push:\n    branches-ignore: ['dependabot/**']", Event{Name: "push", Ref: "refs/heads/dependabot/npm/x"}, false},
		{
			"paths match one file", "  push:\n    paths: ['**.go']",
			Event{Name: "push", Ref: "refs/heads/main", ChangedFiles: []string{"README.md", "cmd/main.go"}}, true,
		},
		{
			"paths match no file", "  push:\n    paths: ['**.go']",
			Event{Name: "push", Ref: "refs/heads/main", ChangedFiles: []string{"README.md"}}, false,
		},
		{
			"paths are not checked for tags", "  push:\n    paths: ['**.go']",
			Event{Name: "push", Ref: "refs/tags/v1", ChangedFiles: []string{"README.md"}}, true,
		},
		{
			"paths pass when the list is truncated", "  push:\n    paths: ['**.go']",
			Event{Name: "push", Ref: "refs/heads/main", ChangedFiles: []string{"README.md"}, ChangedFilesTruncated: true}, true,
		},
		{
			"paths-ignore with one file left", "  push:\n    paths-ignore: ['docs/**']",
			Event{Name: "push", Ref: "refs/heads/main", ChangedFiles: []string{"docs/a.md", "main.go"}}, true,
		},
		{
			"paths-ignore with every file ignored", "  push:\n    paths-ignore: ['docs/**']",
			Event{Name: "push", Ref: "refs/heads/main", ChangedFiles: []string{"docs/a.md", "docs/b.md"}}, false,
		},
		{
			"branches and paths must both match", "  push:\n    branches: [main]\n    paths: ['**.go']",
			Event{Name: "push", Ref: "refs/heads/dev", ChangedFiles: []string{"a.go"}}, false,
		},
		{"pull_request default types", "  pull_request:", Event{Name: "pull_request", Action: "synchronize", Ref: "refs/heads/main"}, true},
		{"pull_request other type", "  pull_request:", Event{Name: "pull_request", Action: "labeled", Ref: "refs/heads/main"}, false},
		{"pull_request listed type", "  pull_request:\n    types: [labeled]", Event{Name: "pull_request", Action: "labeled", Ref: "refs/heads/main"}, true},
		{"pull_request base branch", "  pull_request:\n    branches: [main]", Event{Name: "pull_request", Action: "opened", Ref: "refs/heads/dev"}, false},
		{"issues any type", "  issues:", Event{Name: "issues", Action: "closed"}, true},
		{"schedule", "  schedule:\n    - cron: '0 0 * * *'", Event{Name: "schedule", Cron: "0 0 * * *"}, true},
		{"other schedule", "  schedule:\n    - cron: '0 0 * * *'", Event{Name: "schedule", Cron: "0 1 * * *"}, false},
		{
			"schedule with timezone", "  schedule:\n    - cron: '0 9 * * *'\n      timezone: Europe/Paris",
			Event{Name: "schedule", Cron: "0 9 * * *", Timezone: "Europe/Paris"}, true,
		},
		{
			"same cron in another timezone", "  schedule:\n    - cron: '0 9 * * *'\n      timezone: Europe/Paris",
			Event{Name: "schedule", Cron: "0 9 * * *"}, false,
		},
		{"workflow_run", "  workflow_run:\n    workflows: [Build]\n    types: [completed]", Event{Name: "workflow_run", Workflow: "Build", Action: "completed", Ref: "refs/heads/main"}, true},
		{"workflow_run other workflow", "  workflow_run:\n    workflows: [Build]", Event{Name: "workflow_run", Workflow: "Lint", Action: "completed", Ref: "refs/heads/main"}, false},
		{"workflow_call is never matched", "  workflow_call:", Event{Name: "workflow_call"}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Match(parse(test.on), &test.ev)
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestWorkflowSource(t *testing.T) {
	assert.Equal(t, SourceEventCommit, WorkflowSource("push"))
	assert.Equal(t, SourceMergeCommit, WorkflowSource("pull_request"))
	assert.Equal(t, SourceBaseBranch, WorkflowSource("pull_request_target"))
	assert.Equal(t, SourceDispatchRef, WorkflowSource("workflow_dispatch"))
	assert.Equal(t, SourceDefaultBranch, WorkflowSource("schedule"))
	assert.Equal(t, SourceDefaultBranch, WorkflowSource("issue_comment"))
}
