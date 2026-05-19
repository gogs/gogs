package email

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/testx"
)

// TestRenderEmbeddedTemplates ensures every builtin mail template parses and
// executes against the data shape its production caller supplies, so a syntax
// regression or missing field is caught at build time, not on the first email.
func TestRenderEmbeddedTemplates(t *testing.T) {
	conf.SetMockApp(t, conf.AppOpts{BrandName: "Gogs"})
	conf.SetMockServer(t, conf.ServerOpts{
		ExternalURL:        "https://example.test/",
		LoadAssetsFromDisk: false,
	})
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
	conf.SetMockServer(t, conf.ServerOpts{LoadAssetsFromDisk: false})
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

// The helper tests below run inside a subprocess so they can install fresh
// values for GOGS_WORK_DIR and GOGS_CUSTOM before conf.WorkDir / conf.CustomDir
// memoize via sync.Once. The parent test sets up a temp directory tree and
// invokes the helper via testx.Exec, mirroring the pattern used in
// internal/conf/computed_test.go.

// TestRenderFromDiskHelper exercises the LoadAssetsFromDisk=true path:
//   - primary template loads from <work>/templates/mail.
//   - <custom>/templates/mail overrides the primary copy when present.
//   - templates reload on every render call, so an edit to the custom file
//     between two renders is observed without restarting the process.
//   - removing the custom file falls back to the primary copy.
func TestRenderFromDiskHelper(t *testing.T) {
	if !testx.WantHelperProcess() {
		return
	}

	workDir := os.Getenv("GOGS_WORK_DIR")
	customDir := os.Getenv("GOGS_CUSTOM")

	conf.SetMockApp(t, conf.AppOpts{BrandName: "Gogs"})
	conf.SetMockServer(t, conf.ServerOpts{
		ExternalURL:        "https://example.test/",
		LoadAssetsFromDisk: true,
	})

	const tplPath = "auth/activate"
	primary := filepath.Join(workDir, "templates", "mail", tplPath+".tmpl")
	custom := filepath.Join(customDir, "templates", "mail", tplPath+".tmpl")

	require.NoError(t, os.MkdirAll(filepath.Dir(primary), 0o755))
	require.NoError(t, os.WriteFile(primary, []byte("PRIMARY {{.Username}}"), 0o644))

	body, err := render(tplPath, map[string]any{"Username": "alice"})
	require.NoError(t, err)
	require.Equal(t, "PRIMARY alice", body)

	require.NoError(t, os.MkdirAll(filepath.Dir(custom), 0o755))
	require.NoError(t, os.WriteFile(custom, []byte("CUSTOM v1 {{.Username}}"), 0o644))

	body, err = render(tplPath, map[string]any{"Username": "alice"})
	require.NoError(t, err)
	require.Equal(t, "CUSTOM v1 alice", body)

	require.NoError(t, os.WriteFile(custom, []byte("CUSTOM v2 {{.Username}}"), 0o644))

	body, err = render(tplPath, map[string]any{"Username": "alice"})
	require.NoError(t, err)
	require.Equal(t, "CUSTOM v2 alice", body, "hot-reload should pick up edits without a restart")

	require.NoError(t, os.Remove(custom))

	body, err = render(tplPath, map[string]any{"Username": "alice"})
	require.NoError(t, err)
	require.Equal(t, "PRIMARY alice", body, "removing the custom override should fall back to the primary copy")

	fmt.Fprintln(os.Stdout, "ok")
}

func TestRenderFromDisk(t *testing.T) {
	workDir := t.TempDir()
	customDir := t.TempDir()
	out, err := testx.Exec("TestRenderFromDiskHelper",
		"GOGS_WORK_DIR="+workDir,
		"GOGS_CUSTOM="+customDir,
	)
	require.NoError(t, err, out)
	assert.Equal(t, "ok", out)
}

// TestRenderMissingDiskRootHelper asserts that booting with
// LoadAssetsFromDisk=true but no <work>/templates/mail directory fails fast on
// first render with a wrapped ErrNotExist, rather than silently returning an
// empty template set.
func TestRenderMissingDiskRootHelper(t *testing.T) {
	if !testx.WantHelperProcess() {
		return
	}

	conf.SetMockServer(t, conf.ServerOpts{LoadAssetsFromDisk: true})

	_, err := render("auth/activate", nil)
	if err == nil {
		fmt.Fprintln(os.Stdout, "expected error, got nil")
		os.Exit(1)
	}
	if !strings.Contains(err.Error(), "stat base mail templates") {
		fmt.Fprintf(os.Stdout, "unexpected error: %v", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout, "ok")
}

func TestRenderMissingDiskRoot(t *testing.T) {
	emptyDir := t.TempDir()
	out, err := testx.Exec("TestRenderMissingDiskRootHelper",
		"GOGS_WORK_DIR="+emptyDir,
		"GOGS_CUSTOM="+emptyDir,
	)
	require.NoError(t, err, out)
	assert.Equal(t, "ok", out)
}
