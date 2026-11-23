package repox

import (
	"fmt"
	"path/filepath"
	"strings"

	"gogs.io/gogs/internal/pathx"
)

// CloneLink represents different types of clone URLs of repository.
type CloneLink struct {
	SSH   string
	HTTPS string
}

// CloneLinkOptions are the settings that clone URLs are built from.
type CloneLinkOptions struct {
	// ExternalURL is the external URL of the server, ending with "/".
	ExternalURL string
	// RunUser is the user that SSH clients connect as.
	RunUser   string
	SSHDomain string
	SSHPort   int
}

// NewCloneLink returns clone URLs using given owner and repository name.
func NewCloneLink(opts CloneLinkOptions, owner, repo string, isWiki bool) *CloneLink {
	if isWiki {
		repo += ".wiki"
	}

	cl := new(CloneLink)
	if opts.SSHPort != 22 {
		cl.SSH = fmt.Sprintf("ssh://%s@%s:%d/%s/%s.git", opts.RunUser, opts.SSHDomain, opts.SSHPort, owner, repo)
	} else {
		cl.SSH = fmt.Sprintf("%s@%s:%s/%s.git", opts.RunUser, opts.SSHDomain, owner, repo)
	}
	cl.HTTPS = HTTPSCloneURL(opts.ExternalURL, owner, repo)
	return cl
}

// HTTPSCloneURL returns HTTPS clone URL using the external URL of the server,
// and given owner and repository name.
func HTTPSCloneURL(externalURL, owner, repo string) string {
	return fmt.Sprintf("%s%s/%s.git", externalURL, owner, repo)
}

// HTMLURL returns HTML URL using the external URL of the server, and given
// owner and repository name.
func HTMLURL(externalURL, owner, repo string) string {
	return externalURL + owner + "/" + repo
}

// CompareCommitsPath returns the comparison path using given owner, repository,
// and commit IDs.
func CompareCommitsPath(owner, repo, oldCommitID, newCommitID string) string {
	return fmt.Sprintf("%s/%s/compare/%s...%s", owner, repo, oldCommitID, newCommitID)
}

// UserPath returns the absolute path for storing user repositories under the
// repository root.
func UserPath(root, user string) string {
	// 🚨 SECURITY: Prevent path traversal in user name.
	return filepath.Join(root, pathx.Clean(strings.ToLower(user)))
}

// RepositoryPath returns the absolute path under the repository root using
// given user and repository name.
func RepositoryPath(root, owner, repo string) string {
	// 🚨 SECURITY: Prevent path traversal in repository name.
	return filepath.Join(UserPath(root, owner), pathx.Clean(strings.ToLower(repo))+".git")
}
