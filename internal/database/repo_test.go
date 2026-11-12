package database

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

		// Should not have format and style if no external tracker is configured
		_, ok := metas["format"]
		assert.False(t, ok)
		_, ok = metas["style"]
		assert.False(t, ok)
	})

	t.Run("an external issue tracker is configured", func(t *testing.T) {
		repo.ExternalMetas = nil
		repo.EnableExternalTracker = true

		// Default to numeric issue style
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

	_, err = CreateRepository(owner, owner, opts)
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

func TestPointHEADToBranchIfMissing(t *testing.T) {
	ctx := t.Context()
	git := func(t *testing.T, dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, string(output))
		return strings.TrimSpace(string(output))
	}
	// newRepo returns a bare repository with HEAD pointing to "main" and the
	// given branches.
	newRepo := func(t *testing.T, branches ...string) string {
		t.Helper()
		dir := t.TempDir()
		git(t, dir, "init", "--bare", "--initial-branch=main")
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

	t.Run("default branch is missing", func(t *testing.T) {
		dir := newRepo(t, "dev", "master")

		changed, err := pointHEADToBranchIfMissing(ctx, dir, "main", "master")
		require.NoError(t, err)
		assert.True(t, changed)
		assert.Equal(t, "refs/heads/master", git(t, dir, "symbolic-ref", "HEAD"))
	})

	t.Run("default branch exists", func(t *testing.T) {
		dir := newRepo(t, "main", "master")

		changed, err := pointHEADToBranchIfMissing(ctx, dir, "main", "master")
		require.NoError(t, err)
		assert.False(t, changed)
		assert.Equal(t, "refs/heads/main", git(t, dir, "symbolic-ref", "HEAD"))
	})

	t.Run("candidate branch is missing", func(t *testing.T) {
		dir := newRepo(t, "dev")

		changed, err := pointHEADToBranchIfMissing(ctx, dir, "main", "master")
		require.NoError(t, err)
		assert.False(t, changed)
		assert.Equal(t, "refs/heads/main", git(t, dir, "symbolic-ref", "HEAD"))
	})

	t.Run("empty repository", func(t *testing.T) {
		dir := newRepo(t)

		changed, err := pointHEADToBranchIfMissing(ctx, dir, "main", "master")
		require.NoError(t, err)
		assert.False(t, changed)
		assert.Equal(t, "refs/heads/main", git(t, dir, "symbolic-ref", "HEAD"))
	})
}
