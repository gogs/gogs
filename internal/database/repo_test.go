package database

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/osx"
)

func TestRepository_ComposeMetas(t *testing.T) {
	repo := &Repository{
		Name: "testrepo",
		Owner: &User{
			Name: "testuser",
		},
		ExternalTrackerFormat: "https://someurl.com/{user}/{repo}/{issue}",
	}

	t.Run("no external tracker is configured", func(t *testing.T) {
		repo.EnableExternalTracker = false

		metas := repo.ComposeMetas()
		assert.Equal(t, metas["repoLink"], repo.Link())

		// Should not have format and style if no external tracker is configured.
		_, ok := metas["format"]
		assert.False(t, ok)
		_, ok = metas["style"]
		assert.False(t, ok)
	})

	t.Run("an external issue tracker is configured", func(t *testing.T) {
		repo.ExternalMetas = nil
		repo.EnableExternalTracker = true

		// Default to numeric issue style.
		assert.Equal(t, markup.IssueNameStyleNumeric, repo.ComposeMetas()["style"])
		repo.ExternalMetas = nil

		repo.ExternalTrackerStyle = markup.IssueNameStyleNumeric
		assert.Equal(t, markup.IssueNameStyleNumeric, repo.ComposeMetas()["style"])
		repo.ExternalMetas = nil

		repo.ExternalTrackerStyle = markup.IssueNameStyleAlphanumeric
		assert.Equal(t, markup.IssueNameStyleAlphanumeric, repo.ComposeMetas()["style"])
		repo.ExternalMetas = nil

		metas := repo.ComposeMetas()
		assert.Equal(t, "testuser", metas["user"])
		assert.Equal(t, "testrepo", metas["repo"])
		assert.Equal(t, "https://someurl.com/{user}/{repo}/{issue}", metas["format"])
	})
}

func Test_CreateRepository_PreventDeletion(t *testing.T) {
	tempRepositoryRoot := filepath.Join(os.TempDir(), "createRepository-tempRepositoryRoot")
	conf.SetMockRepository(
		t,
		conf.RepositoryOpts{
			Root: tempRepositoryRoot,
		},
	)
	err := os.RemoveAll(tempRepositoryRoot)
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempRepositoryRoot) }()

	owner := &User{Name: "testuser"}
	opts := CreateRepoOptionsLegacy{Name: "safety-test"}
	repoPath := RepoPath(owner.Name, opts.Name)
	require.NoError(t, os.MkdirAll(repoPath, os.ModePerm))

	canary := filepath.Join(repoPath, "canary.txt")
	require.NoError(t, os.WriteFile(canary, []byte("should survive"), 0o644))

	// No database is needed, because the existing directory stops creation first.
	_, err = newReposStore(nil).Create(context.Background(), logx.NewNoopLogger(), owner, owner, opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repository directory already exists")
	assert.True(t, osx.Exist(canary))
}

func TestGetRepoInitFile(t *testing.T) {
	before := Readmes
	t.Cleanup(func() { Readmes = before })
	Readmes = []string{"Default"}

	data, err := getRepoInitFile("readme", "Default")
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	for _, name := range []string{
		"",
		"../../app.ini",
		`..\app.ini`,
		`Default\..\..\app.ini`,
		"Default/../../app.ini",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := getRepoInitFile("readme", name)
			assert.Error(t, err)
		})
	}

	// Only names of known templates are accepted, even when the file exists.
	Readmes = nil
	_, err = getRepoInitFile("readme", "Default")
	assert.Error(t, err)
}

func TestResolveMissingDefaultBranch(t *testing.T) {
	ctx := t.Context()
	git := func(t *testing.T, dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, string(output))
		return strings.TrimSpace(string(output))
	}
	// newRepo returns a bare repository with HEAD pointing to the given branch and
	// having the given branches.
	newRepo := func(t *testing.T, head string, branches ...string) string {
		t.Helper()
		dir := t.TempDir()
		git(t, dir, "init", "--bare", "--initial-branch="+head)
		if len(branches) == 0 {
			return dir
		}

		work := t.TempDir()
		git(t, work, "init", "--initial-branch="+branches[0])
		git(t, work, "-c", "user.name=alice", "-c", "user.email=alice@example.com", "commit", "--allow-empty", "--message=initial")
		for _, branch := range branches {
			git(t, work, "push", dir, "HEAD:refs/heads/"+branch)
		}
		return dir
	}

	tests := []struct {
		name       string
		head       string
		branches   []string
		saved      string
		candidate  string
		wantBranch string
		wantHEAD   string
	}{
		{
			name:       "first push uses a different branch name",
			head:       "main",
			branches:   []string{"dev", "master"},
			saved:      "main",
			candidate:  "master",
			wantBranch: "master",
			wantHEAD:   "refs/heads/master",
		},
		{
			name:       "saved default branch exists",
			head:       "main",
			branches:   []string{"main", "master"},
			saved:      "main",
			candidate:  "master",
			wantBranch: "",
			wantHEAD:   "refs/heads/main",
		},
		{
			name:       "no default branch saved and HEAD branch exists",
			head:       "master",
			branches:   []string{"master", "dev"},
			saved:      "",
			candidate:  "dev",
			wantBranch: "master",
			wantHEAD:   "refs/heads/master",
		},
		{
			name:       "saved default branch is missing and HEAD branch exists",
			head:       "trunk",
			branches:   []string{"trunk", "dev"},
			saved:      "main",
			candidate:  "dev",
			wantBranch: "trunk",
			wantHEAD:   "refs/heads/trunk",
		},
		{
			name:       "candidate branch is missing",
			head:       "main",
			branches:   []string{"dev"},
			saved:      "main",
			candidate:  "master",
			wantBranch: "",
			wantHEAD:   "refs/heads/main",
		},
		{
			name:       "empty repository",
			head:       "main",
			saved:      "main",
			candidate:  "master",
			wantBranch: "",
			wantHEAD:   "refs/heads/main",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := newRepo(t, test.head, test.branches...)

			got, err := resolveMissingDefaultBranch(ctx, dir, test.saved, test.candidate)
			require.NoError(t, err)
			assert.Equal(t, test.wantBranch, got)
			assert.Equal(t, test.wantHEAD, git(t, dir, "symbolic-ref", "HEAD"))
		})
	}
}

func TestSyncDelegateHooks(t *testing.T) {
	hookNames := func(repoPath string) []string {
		var names []string
		for _, name := range []string{"pre-receive", "update", "post-receive"} {
			if osx.IsFile(filepath.Join(repoPath, "hooks", name)) {
				names = append(names, name)
			}
		}
		return names
	}

	tests := []struct {
		name               string
		customHooks        []string
		hasProtectedBranch bool
		want               []string
	}{
		{name: "nothing to do"},
		{name: "protected branch", hasProtectedBranch: true, want: []string{"pre-receive"}},
		{name: "custom update hook", customHooks: []string{"update"}, want: []string{"update"}},
		{
			name:               "custom post-receive hook and protected branch",
			customHooks:        []string{"post-receive"},
			hasProtectedBranch: true,
			want:               []string{"pre-receive", "post-receive"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repoPath := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(repoPath, "hooks"), 0o755))
			require.NoError(t, os.MkdirAll(filepath.Join(repoPath, "custom_hooks"), 0o755))
			// Delegate hooks from before are removed when no longer needed.
			for _, name := range []string{"pre-receive", "update", "post-receive"} {
				require.NoError(t, os.WriteFile(filepath.Join(repoPath, "hooks", name), nil, 0o755))
			}
			for _, name := range test.customHooks {
				require.NoError(t, os.WriteFile(filepath.Join(repoPath, "custom_hooks", name), nil, 0o755))
			}

			require.NoError(t, syncDelegateHooks(repoPath, func() (bool, error) { return test.hasProtectedBranch, nil }))
			assert.Equal(t, test.want, hookNames(repoPath))
		})
	}
}

func TestSyncDelegateHooks_ProtectedMeanwhile(t *testing.T) {
	repoPath := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repoPath, "hooks"), 0o755))
	hookPath := filepath.Join(repoPath, "hooks", "pre-receive")
	require.NoError(t, os.WriteFile(hookPath, nil, 0o755))

	// A branch is protected, e.g., in another process, after the sync first read
	// that none was, and before it removed the hook.
	checks := 0
	hasProtectedBranch := func() (bool, error) {
		checks++
		return checks > 1, nil
	}
	require.NoError(t, syncDelegateHooks(repoPath, hasProtectedBranch))
	assert.True(t, osx.IsFile(hookPath), "pre-receive hook must stay installed")
}

func TestRepository_limitVisibilityTo(t *testing.T) {
	public := Repository{}
	unlisted := Repository{IsUnlisted: true}
	private := Repository{IsPrivate: true}
	tests := []struct {
		name string
		fork Repository
		base Repository
		want Repository
	}{
		{name: "public fork of a private base", fork: public, base: private, want: private},
		{name: "unlisted fork of a private base", fork: unlisted, base: private, want: Repository{IsPrivate: true, IsUnlisted: true}},
		{name: "public fork of an unlisted base", fork: public, base: unlisted, want: unlisted},
		{name: "private fork of a public base stays private", fork: private, base: public, want: private},
		{name: "unlisted fork of a public base stays unlisted", fork: unlisted, base: public, want: unlisted},
		{name: "same visibility", fork: public, base: public, want: public},
		{
			name: "public wiki and issues of a private fork of a private base that allows neither",
			fork: Repository{IsPrivate: true, AllowPublicWiki: true, AllowPublicIssues: true},
			base: private,
			want: private,
		},
		{
			name: "public issues of a private fork of a private base that allows only them",
			fork: Repository{IsPrivate: true, AllowPublicWiki: true, AllowPublicIssues: true},
			base: Repository{IsPrivate: true, AllowPublicIssues: true},
			want: Repository{IsPrivate: true, AllowPublicIssues: true},
		},
		{
			name: "public wiki and issues of a private fork of a public base",
			fork: Repository{IsPrivate: true, AllowPublicWiki: true, AllowPublicIssues: true},
			base: public,
			want: Repository{IsPrivate: true, AllowPublicWiki: true, AllowPublicIssues: true},
		},
		{
			// Explore would list the fork but not its base.
			name: "public wiki and issues of a private fork of an unlisted base",
			fork: Repository{IsPrivate: true, AllowPublicWiki: true, AllowPublicIssues: true},
			base: unlisted,
			want: Repository{IsPrivate: true, IsUnlisted: true},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fork := test.fork
			fork.limitVisibilityTo(&test.base)
			assert.Equal(t, test.want.IsPrivate, fork.IsPrivate, "IsPrivate")
			assert.Equal(t, test.want.IsUnlisted, fork.IsUnlisted, "IsUnlisted")
			assert.Equal(t, test.want.AllowPublicWiki, fork.AllowPublicWiki, "AllowPublicWiki")
			assert.Equal(t, test.want.AllowPublicIssues, fork.AllowPublicIssues, "AllowPublicIssues")
		})
	}
}
