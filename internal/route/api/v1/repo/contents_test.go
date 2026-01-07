package repo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/pathutil"
	"gogs.io/gogs/internal/repoutil"
)

// TestValidateRepoPathSymlink ensures that repository path validation
// detects symlink traversal. This validation is used by PutContents.
func TestValidateRepoPathSymlink(t *testing.T) {
	if os.Getenv("GO_WANT_DEBUGGER") == "1" {
		t.Skip("skipping under debugger")
	}

	// Setup mock repository root under a temp directory
	repoRoot := t.TempDir()
	conf.SetMockRepository(t, conf.RepositoryOpts{Root: repoRoot})

	owner := "alice"
	repo := "example"
	repoPath := repoutil.RepositoryPath(owner, repo)

	// create repo directory (permissions must be 0750 or less)
	if err := os.MkdirAll(repoPath, 0o750); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}

	// create an external target outside the repo root
	external := t.TempDir()
	targetFile := filepath.Join(external, "outside.txt")
	if err := os.WriteFile(targetFile, []byte("malicious"), 0o600); err != nil {
		t.Fatalf("failed to write external file: %v", err)
	}

	// create a symlink inside the repo that points to the external file
	linkPath := filepath.Join(repoPath, "malicious_link")
	if err := os.Symlink(targetFile, linkPath); err != nil {
		// On some platforms symlink may require privileges; skip if not supported
		t.Skipf("symlink not supported: %v", err)
	}

	// ValidatePathWithin should detect symlink traversal when checking the link
	err := repoutil.ValidatePathWithin(repoPath, "malicious_link")
	assert.Error(t, err)
	assert.True(t, pathutil.IsErrSymlinkTraversal(err), "expected symlink traversal error")
}
