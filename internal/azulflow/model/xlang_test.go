package model

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "go.yaml.in/yaml/v3"
)

// xlangOptions are the options of a case of GitHub's conformance suite.
type xlangOptions struct {
	IncludeSource         bool            `yaml:"include-source"`
	Skip                  []string        `yaml:"skip"`
	PermissionsPolicy     string          `yaml:"permissions-policy"`
	MaxNestedDepth        int             `yaml:"max-nested-reusable-workflows-depth"`
	ExperimentalFeatures  map[string]bool `yaml:"experimental-features"`
	MaxDepth              int             `yaml:"max-depth"`
	MaxFileSize           int             `yaml:"max-file-size"`
	MaxResultSize         int             `yaml:"max-result-size"`
	MaxJobLimit           int             `yaml:"max-job-limit"`
	SkipReusableWorkflows bool            `yaml:"skip-reusable-workflows"`
	IsPartialRerun        bool            `yaml:"is-partial-rerun"`
}

// xlangScopes are the permission scopes GitHub had when the suite was
// written, before attestations, artifact-metadata, models, and
// vulnerability-alerts.
var xlangScopes = []string{
	"actions", "checks", "contents", "deployments", "discussions", "issues", "packages",
	"pages", "pull-requests", "repository-projects", "statuses", "security-events", "id-token",
}

// xlangDeviations are the cases whose expected result Azulflow deliberately
// does not produce, with the reason.
var xlangDeviations = map[string]string{
	"is-partial-rerun.yml": "it expects the file table of a GitHub bug that a feature flag fixes, and the case is to be removed with the flag",
}

type mapFiles map[string]string

func (m mapFiles) ReadFile(name string) (string, error) {
	content, ok := m[name]
	if !ok {
		return "", errors.Newf("File not found: %s", name)
	}
	return content, nil
}

func (m mapFiles) ResolveRef(caller, ref string) (string, error) {
	return "", errors.Newf("no repository for %s", caller)
}

func TestXLang(t *testing.T) {
	paths, err := filepath.Glob("testdata/reader/*.yml")
	require.NoError(t, err)
	require.Len(t, paths, 195)

	split := regexp.MustCompile(`\r?\n---\r?\n`)
	for _, path := range paths {
		file := filepath.Base(path)
		t.Run(file, func(t *testing.T) {
			if reason, ok := xlangDeviations[file]; ok {
				t.Skip("deliberate deviation: " + reason)
			}

			data, err := os.ReadFile(path)
			require.NoError(t, err)
			docs := split.Split(string(data), -1)
			require.GreaterOrEqual(t, len(docs), 3)

			var o xlangOptions
			require.NoError(t, yaml.Unmarshal([]byte(docs[0]), &o))

			name := ".github/workflows/" + file
			files := mapFiles{name: docs[1]}
			for i := 2; i+1 < len(docs)-1; i += 2 {
				files[docs[i]] = docs[i+1]
			}

			opts := DefaultOptions()
			opts.AllowAnchors = false
			opts.Scopes = xlangScopes
			// The cases only the TypeScript parser runs expect the errors of
			// GitHub's editor tooling.
			typeScriptOnly := containsFold(o.Skip, "Go") && containsFold(o.Skip, "C#")
			opts.StrictEvents = typeScriptOnly
			opts.BackgroundSteps = o.ExperimentalFeatures["allowBackgroundSteps"]
			opts.SkipReusableWorkflows = o.SkipReusableWorkflows
			opts.MaxJobs = o.MaxJobLimit
			if o.PermissionsPolicy != "" {
				opts.PermissionsPolicy = o.PermissionsPolicy
			}
			if o.MaxNestedDepth > 0 {
				opts.MaxNestedDepth = o.MaxNestedDepth
			}
			if o.MaxDepth > 0 {
				opts.Limits.MaxDepth = o.MaxDepth
			}
			if o.MaxFileSize > 0 {
				opts.MaxFileSize = o.MaxFileSize
			}
			if o.MaxResultSize > 0 {
				opts.Limits.MaxBytes = o.MaxResultSize
			}

			w, err := Load(files, name, opts)
			require.NoError(t, err)

			// As in GitHub's harness, events are only compared in the cases
			// that only the TypeScript parser runs, whose template has events
			// but not input types.
			if typeScriptOnly {
				w.InputTypes = nil
			} else {
				w.Events = nil
			}
			assert.Equal(t, strings.TrimSpace(docs[len(docs)-1]), w.JSON(o.IncludeSource))
		})
	}
}

func containsFold(list []string, s string) bool {
	for _, item := range list {
		if strings.EqualFold(item, s) {
			return true
		}
	}
	return false
}
