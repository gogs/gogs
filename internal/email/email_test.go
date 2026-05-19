package email

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/conf"
)

// TestRenderEmbeddedTemplates ensures every builtin mail template parses and
// executes against the data shape its production caller supplies, so a syntax
// regression or missing field is caught at build time, not on the first email.
func TestRenderEmbeddedTemplates(t *testing.T) {
	conf.App.BrandName = "Gogs"
	conf.Server.ExternalURL = "https://example.test/"
	conf.Server.LoadAssetsFromDisk = false

	resetTemplateCache(t)

	tests := []struct {
		name string
		data map[string]any
	}{
		{
			name: tmplAuthActivate,
			data: map[string]any{
				"Username":          "alice",
				"ActiveCodeLives":   1440,
				"ResetPwdCodeLives": 1440,
				"Code":              "abc",
			},
		},
		{
			name: tmplAuthActivateEmail,
			data: map[string]any{
				"Username":        "alice",
				"ActiveCodeLives": 1440,
				"Code":            "abc",
				"Email":           "alice@example.test",
			},
		},
		{
			name: tmplAuthResetPassword,
			data: map[string]any{
				"Username":          "alice",
				"ActiveCodeLives":   1440,
				"ResetPwdCodeLives": 1440,
				"Code":              "abc",
			},
		},
		{
			name: tmplAuthRegisterNotify,
			data: map[string]any{"Username": "alice"},
		},
		{
			name: tmplNotifyCollaborator,
			data: map[string]any{
				"Subject":  "alice added you to bob/repo",
				"RepoName": "bob/repo",
				"Link":     "https://example.test/bob/repo",
			},
		},
		{
			name: tmplIssueComment,
			data: map[string]any{
				"Subject": "[bob/repo] Re: Issue title",
				"Body":    "<p>comment body</p>",
				"Link":    "https://example.test/bob/repo/issues/1",
				"Doer":    testDoer{},
			},
		},
		{
			name: tmplIssueMention,
			data: map[string]any{
				"Subject": "[bob/repo] @alice mentioned you",
				"Body":    "<p>mention body</p>",
				"Link":    "https://example.test/bob/repo/issues/1",
				"Doer":    testDoer{},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, err := render(tc.name, tc.data)
			require.NoError(t, err)
			assert.NotEmpty(t, body)
			assert.False(t, strings.Contains(body, "<no value>"), "template referenced a missing data key")
		})
	}
}

// TestRenderUnknownTemplate asserts callers get a useful error rather than an
// empty body when asking for a name that doesn't exist.
func TestRenderUnknownTemplate(t *testing.T) {
	conf.Server.LoadAssetsFromDisk = false
	resetTemplateCache(t)

	_, err := render("does/not/exist", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// resetTemplateCache forces the next render call to reload templates, so each
// test starts from a clean state regardless of execution order.
func resetTemplateCache(t *testing.T) {
	t.Helper()
	tplSet = nil
	tplSetErr = nil
	tplSetOnce = sync.Once{}
}

// testDoer satisfies the User interface for fields the issue templates touch.
type testDoer struct{}

func (testDoer) ID() int64                               { return 1 }
func (testDoer) DisplayName() string                     { return "alice" }
func (testDoer) Email() string                           { return "alice@example.test" }
func (testDoer) GenerateEmailActivateCode(string) string { return "abc" }
