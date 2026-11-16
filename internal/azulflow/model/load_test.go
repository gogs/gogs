package model

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// GitHub accepts YAML anchors and aliases, which its conformance suite only
// covers with anchors disabled.
func TestLoadAnchors(t *testing.T) {
	files := mapFiles{"w.yml": `on: push
jobs:
  a:
    runs-on: &os ubuntu-latest
    steps: &steps
      - run: echo hi
  b:
    runs-on: *os
    steps: *steps
`}
	w, err := Load(files, "w.yml", DefaultOptions())
	require.NoError(t, err)
	require.Empty(t, w.Errors)
	require.Len(t, w.Jobs, 2)
	b := w.Jobs[1]
	assert.Equal(t, "ubuntu-latest", b.RunsOn.Str)
	require.Len(t, b.Steps, 1)
	assert.Equal(t, "echo hi", b.Steps[0].Run.Str)

	opts := DefaultOptions()
	opts.AllowAnchors = false
	w, err = Load(files, "w.yml", opts)
	require.NoError(t, err)
	assert.Equal(t, []string{"w.yml: Anchors are not currently supported. Remove the anchor 'os'"}, w.Errors)
}

type repoFiles map[string]string

func (f repoFiles) ReadFile(name string) (string, error) {
	return mapFiles(f).ReadFile(name)
}

func (repoFiles) ResolveRef(_, ref string) (string, error) {
	return "o/r/" + ref[2:] + "@sha", nil
}

// GitHub resolves a reusable workflow "$/{path}" in the repository of the
// run, as it does "./{path}", and accepts background steps.
func TestLoadSelfRepositoryWorkflowAndBackgroundSteps(t *testing.T) {
	files := repoFiles{
		"o/r/.github/workflows/ci.yml@sha": `on: push
jobs:
  a:
    uses: $/.github/workflows/build.yml
`,
		"o/r/.github/workflows/build.yml@sha": `on: workflow_call
jobs:
  b:
    runs-on: x
    steps:
      - id: server
        run: ./serve
        background: true
      - run: ./test
      - wait: [server]
`,
	}
	w, err := Load(files, "o/r/.github/workflows/ci.yml@sha", DefaultOptions())
	require.NoError(t, err)
	require.Empty(t, w.Errors)
	a := w.Jobs[0]
	assert.Equal(t, "o/r/.github/workflows/build.yml@sha", a.Ref.Str)
	require.Len(t, a.Jobs, 1)
	steps := a.Jobs[0].Steps
	require.Len(t, steps, 3)
	assert.True(t, *steps[0].Background)
	assert.Equal(t, "server", steps[2].Wait[0].Str)
}

// GitHub connects at most ten levels of workflows: the caller and nine
// levels of reusable workflows.
func TestLoadNestedDepth(t *testing.T) {
	chain := func(levels int) repoFiles {
		files := repoFiles{"o/r/.github/workflows/ci.yml@sha": "on: push\njobs:\n  call:\n    uses: ./.github/workflows/1.yml\n"}
		for i := 1; i < levels; i++ {
			next := fmt.Sprintf("    uses: ./.github/workflows/%d.yml\n", i+1)
			if i == levels-1 {
				next = "    runs-on: x\n    steps:\n      - run: echo\n"
			}
			files[fmt.Sprintf("o/r/.github/workflows/%d.yml@sha", i)] = "on: workflow_call\njobs:\n  call:\n" + next
		}
		return files
	}
	w, err := Load(chain(10), "o/r/.github/workflows/ci.yml@sha", DefaultOptions())
	require.NoError(t, err)
	assert.Empty(t, w.Errors)

	w, err = Load(chain(11), "o/r/.github/workflows/ci.yml@sha", DefaultOptions())
	require.NoError(t, err)
	assert.Equal(t, []string{"Nested reusable workflow depth exceeded 9."}, w.Errors)
}
