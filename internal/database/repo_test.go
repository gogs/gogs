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

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/lfsx"
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

	before := Handle
	Handle = &DB{db: newTestDB(t, "createRepositoryPreventDeletion")}
	t.Cleanup(func() { Handle = before })

	owner := &User{ID: 1, Name: "testuser"}
	opts := CreateRepoOptionsLegacy{Name: "safety-test"}
	repoPath := RepoPath(owner.Name, opts.Name)
	require.NoError(t, os.MkdirAll(repoPath, os.ModePerm))

	canary := filepath.Join(repoPath, "canary.txt")
	require.NoError(t, os.WriteFile(canary, []byte("should survive"), 0o644))

	_, err = CreateRepository(context.Background(), owner, owner, opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repository directory already exists")
	assert.True(t, osx.Exist(canary))

	// With the repository in the database, the name is reported as taken.
	require.NoError(t, Handle.db.Create(&Repository{OwnerID: owner.ID, Name: "Safety-Test", LowerName: "safety-test"}).Error)
	_, err = CreateRepository(context.Background(), owner, owner, CreateRepoOptionsLegacy{Name: "Safety-Test"})
	assert.True(t, IsErrRepoAlreadyExist(err), "want ErrRepoAlreadyExist, got %v", err)
	assert.True(t, osx.Exist(canary))
}

func TestForkRepository_CopiesLFSObjects(t *testing.T) {
	conf.SetMockRepository(t, conf.RepositoryOpts{Root: t.TempDir(), MaxCreationLimit: -1})
	before := Handle
	Handle = &DB{db: newTestDB(t, "forkRepositoryCopiesLFSObjects")}
	t.Cleanup(func() { Handle = before })

	ctx := context.Background()
	alice, err := Handle.Users().Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	bob, err := Handle.Users().Create(ctx, "bob", "bob@example.com", CreateUserOptions{})
	require.NoError(t, err)

	base := &Repository{OwnerID: alice.ID, Owner: alice, Name: "base", LowerName: "base"}
	require.NoError(t, Handle.db.Create(base).Error)
	other := &Repository{OwnerID: alice.ID, Owner: alice, Name: "other", LowerName: "other"}
	require.NoError(t, Handle.db.Create(other).Error)
	require.NoError(t, exec.Command("git", "init", "--bare", "--quiet", base.RepoPath()).Run())

	require.NoError(t, Handle.LFS().CreateObject(ctx, base.ID, "aa", 1, lfsx.StorageLocal))
	require.NoError(t, Handle.LFS().CreateObject(ctx, base.ID, "bb", 2, lfsx.StorageLocal))
	require.NoError(t, Handle.LFS().CreateObject(ctx, other.ID, "cc", 3, lfsx.StorageLocal))

	fork, err := ForkRepository(ctx, bob, bob, base, "fork", "")
	require.NoError(t, err)

	objects, err := Handle.LFS().GetObjectsByOIDs(ctx, fork.ID, "aa", "bb", "cc")
	require.NoError(t, err)
	got := make(map[lfsx.OID]int64, len(objects))
	for _, object := range objects {
		got[object.OID] = object.Size
	}
	assert.Equal(t, map[lfsx.OID]int64{"aa": 1, "bb": 2}, got)

	// The fork keeps its records when the base repository is deleted.
	require.NoError(t, DeleteRepository(alice.ID, base.ID))
	objects, err = Handle.LFS().GetObjectsByOIDs(ctx, fork.ID, "aa", "bb")
	require.NoError(t, err)
	assert.Len(t, objects, 2)
	objects, err = Handle.LFS().GetObjectsByOIDs(ctx, base.ID, "aa", "bb")
	require.NoError(t, err)
	assert.Empty(t, objects)
}

func TestSearchRepositoryByName(t *testing.T) {
	before := Handle
	Handle = &DB{db: newTestDB(t, "searchRepositoryByName")}
	t.Cleanup(func() { Handle = before })

	require.NoError(t, Handle.db.Create(&Repository{OwnerID: 1, Name: "doomed", LowerName: "doomed", Description: "Will be deleted"}).Error)

	// Both the name and the description match regardless of case.
	for _, keyword := range []string{"DOOMED", "will"} {
		repos, count, err := SearchRepositoryByName(&SearchRepoOptions{Keyword: keyword, Page: 1, PageSize: 10})
		require.NoError(t, err)
		assert.Equal(t, int64(1), count, keyword)
		assert.Len(t, repos, 1, keyword)
	}
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
