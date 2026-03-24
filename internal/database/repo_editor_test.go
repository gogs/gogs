package database

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	git "github.com/gogs/git-module"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/conf"
)

func TestIsRepositoryGitPath(t *testing.T) {
	tests := []struct {
		path    string
		wantVal bool
	}{
		{path: ".git", wantVal: true},
		{path: "./.git", wantVal: true},
		{path: ".git/hooks/pre-commit", wantVal: true},
		{path: ".git/hooks", wantVal: true},
		{path: "dir/.git", wantVal: true},

		// Case-insensitive file system
		{path: ".Git", wantVal: true},
		{path: "./.Git", wantVal: true},
		{path: ".Git/hooks/pre-commit", wantVal: true},
		{path: ".Git/hooks", wantVal: true},
		{path: "dir/.Git", wantVal: true},

		{path: ".gitignore", wantVal: false},
		{path: "dir/.gitkeep", wantVal: false},

		// Windows-specific
		{path: `.git\`, wantVal: true},
		{path: `.git\hooks\pre-commit`, wantVal: true},
		{path: `.git\hooks`, wantVal: true},
		{path: `dir\.git`, wantVal: true},

		{path: `.\.git.`, wantVal: true},
		{path: `.\.git.\`, wantVal: true},
		{path: `.git.\hooks\pre-commit`, wantVal: true},
		{path: `.git.\hooks`, wantVal: true},
		{path: `dir\.git.`, wantVal: true},

		{path: "./.git.", wantVal: true},
		{path: "./.git./", wantVal: true},
		{path: ".git./hooks/pre-commit", wantVal: true},
		{path: ".git./hooks", wantVal: true},
		{path: "dir/.git.", wantVal: true},

		{path: `dir\.gitkeep`, wantVal: false},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			assert.Equal(t, test.wantVal, isRepositoryGitPath(test.path))
		})
	}
}

func TestRepository_UpdateRepoFile_ExistingBranch(t *testing.T) {
	tmpDir := t.TempDir()
	repoRoot := filepath.Join(tmpDir, "repos")
	appData := filepath.Join(tmpDir, "appdata")

	require.NoError(t, os.MkdirAll(repoRoot, 0o755))
	require.NoError(t, os.MkdirAll(appData, 0o755))

	conf.SetMockRepository(t, conf.RepositoryOpts{Root: repoRoot})
	conf.SetMockServer(t, conf.ServerOpts{AppDataPath: appData})

	// Set up a working (non-bare) repo to create commits and branches.
	workPath := filepath.Join(tmpDir, "work")
	require.NoError(t, git.Init(workPath))
	require.NoError(t, exec.Command("git", "-C", workPath, "config", "user.email", "test@example.com").Run())
	require.NoError(t, exec.Command("git", "-C", workPath, "config", "user.name", "Test User").Run())

	sig := &git.Signature{Name: "Test User", Email: "test@example.com", When: time.Now()}

	// Create initial commit on master with README.md.
	require.NoError(t, os.WriteFile(filepath.Join(workPath, "README.md"), []byte("# Test"), 0o644))
	require.NoError(t, git.Add(workPath, git.AddOptions{All: true}))
	require.NoError(t, git.CreateCommit(workPath, sig, "Initial commit"))

	// Create dev branch from master with the same content.
	require.NoError(t, git.Checkout(workPath, "dev", git.CheckoutOptions{BaseBranch: "master"}))

	// Init bare repo at the path the Repository struct will use.
	barePath := filepath.Join(repoRoot, "testowner", "testrepo.git")
	require.NoError(t, os.MkdirAll(filepath.Dir(barePath), 0o755))
	require.NoError(t, git.Init(barePath, git.InitOptions{Bare: true}))

	// Push both branches from the working repo to the bare repo.
	require.NoError(t, git.RemoteAdd(workPath, "origin", barePath))
	require.NoError(t, git.Push(workPath, "origin", "master"))
	require.NoError(t, git.Push(workPath, "origin", "dev"))

	repo := &Repository{
		ID:   1,
		Name: "testrepo",
		Owner: &User{
			Name: "testowner",
		},
		DefaultBranch: "master",
	}

	doer := &User{
		Name:     "committer",
		FullName: "Committer",
		Email:    "committer@example.com",
	}

	// Creating a new file on the existing dev branch should succeed without
	// the "branch already exists" error that was triggered by always setting
	// OldBranch to the default branch when the target branch already exists.
	err := repo.UpdateRepoFile(doer, UpdateRepoFileOptions{
		OldBranch:   "dev",
		NewBranch:   "dev",
		OldTreeName: "CONTRIBUTING.md",
		NewTreeName: "CONTRIBUTING.md",
		Message:     "Add CONTRIBUTING.md",
		Content:     "# Contributing",
	})
	require.NoError(t, err)

	// Verify the file was created on the dev branch.
	gitRepo, err := git.Open(barePath)
	require.NoError(t, err)
	commit, err := gitRepo.CatFileCommit("dev")
	require.NoError(t, err)
	entry, err := commit.TreeEntry("CONTRIBUTING.md")
	require.NoError(t, err)
	assert.Equal(t, "CONTRIBUTING.md", entry.Name())
}
