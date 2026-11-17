package integrationtests

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/database"
)

// protectedBranchTeam creates a team of the organization with the access to
// the repository and the members.
func protectedBranchTeam(t *testing.T, org *database.User, repo *Repository, mode database.AccessMode, memberIDs ...int64) *database.Team {
	t.Helper()

	team := &database.Team{OrgID: org.ID, Name: UniqueName("team-"), Authorize: mode}
	require.NoError(t, database.NewTeam(team))
	r, err := getRepository(t, repo.ID)
	require.NoError(t, err)
	require.NoError(t, team.AddRepository(r))
	for _, id := range memberIDs {
		require.NoError(t, team.AddMember(id))
	}
	return team
}

// protectedBranchPush commits a new file on main in a new clone as the user,
// pushes it, and returns the output of the push and whether it succeeded.
func protectedBranchPush(t *testing.T, repo *Repository, u *User) (string, bool) {
	t.Helper()

	dir := gitClone(t, gitRemoteURL(t, repo, u.Name, u.Password))
	gitCommitFile(t, dir, UniqueName("file-")+".txt")
	out, err := gitCommand(dir, "push", "origin", "main").CombinedOutput()
	return string(out), err == nil
}

// protectedBranchMerge merges the pull request as the client and returns the
// status code and the response body.
func protectedBranchMerge(t *testing.T, c *client, repo *Repository, index int64) (int, []byte) {
	t.Helper()

	return c.do(http.MethodPost, issueAPIPath(repo, "pulls", index, "merge"), map[string]any{"style": "create_merge_commit"})
}

func TestProtectedBranchAllowlist(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	listed := seedUser(t, CreateUserOptions{})
	unlisted := seedUser(t, CreateUserOptions{})
	joiner := seedUser(t, CreateUserOptions{})
	reader := seedUser(t, CreateUserOptions{})
	org := orgSeed(t, owner.ID)
	repo := seedRepository(t, org.ID, CreateRepositoryOptions{AutoInit: true})
	protectedBranchTeam(t, org, repo, database.AccessModeWrite, listed.ID, unlisted.ID, joiner.ID)
	release := protectedBranchTeam(t, org, repo, database.AccessModeWrite)
	readers := protectedBranchTeam(t, org, repo, database.AccessModeRead, reader.ID)

	ownerClient := orgSignedIn(t, owner)
	listedClient := orgSignedIn(t, listed)
	unlistedClient := orgSignedIn(t, unlisted)
	joinerClient := orgSignedIn(t, joiner)

	settingsPath := issueAPIPath(repo, "settings", "branches", "main")
	protect := func(t *testing.T, body map[string]any) {
		t.Helper()
		status, resp := ownerClient.do(http.MethodPut, settingsPath, body)
		require.Equal(t, http.StatusNoContent, status, "response: %s", resp)
	}
	// The web editor does not commit from a branch that requires pull
	// requests, so the head branch is pushed.
	newPull := func(t *testing.T) int64 {
		t.Helper()
		branch := UniqueName("feature-")
		dir := gitClone(t, gitRemoteURL(t, repo, owner.Name, owner.Password))
		gitRun(t, dir, "checkout", "-b", branch)
		gitCommitFile(t, dir, branch+".txt")
		gitRun(t, dir, "push", "origin", branch)
		return pullCreate(t, ownerClient, repo, branch, "Change "+branch)
	}

	protect(t, map[string]any{
		"protected":          true,
		"requirePullRequest": true,
		"enableAllowlist":    true,
		"allowlistUserIDs":   []int64{listed.ID, reader.ID},
		"allowlistTeamIDs":   []int64{release.ID, readers.ID},
	})

	t.Run("settings drop people and teams who cannot write", func(t *testing.T) {
		var got struct {
			EnableAllowlist  bool    `json:"enableAllowlist"`
			AllowlistUserIDs []int64 `json:"allowlistUserIDs"`
			AllowlistTeamIDs []int64 `json:"allowlistTeamIDs"`
		}
		ownerClient.doJSON(http.MethodGet, settingsPath, nil, http.StatusOK, &got)
		assert.True(t, got.EnableAllowlist)
		assert.Equal(t, []int64{listed.ID}, got.AllowlistUserIDs)
		assert.Equal(t, []int64{release.ID}, got.AllowlistTeamIDs)
	})

	t.Run("writer outside the allowlist", func(t *testing.T) {
		index := newPull(t)
		v := pullWaitMergeable(t, unlistedClient, repo, index)
		assert.False(t, v.Pull.ViewerCanMerge)
		assert.True(t, v.Pull.ViewerNotInAllowlist)

		before := gitMainOnDisk(t, repo)
		status, body := protectedBranchMerge(t, unlistedClient, repo, index)
		require.Equal(t, http.StatusForbidden, status, "response: %s", body)
		var resp struct {
			Error string `json:"error"`
		}
		require.NoError(t, json.Unmarshal(body, &resp))
		assert.Equal(t, "Only people in the push allowlist of the protected base branch can merge this pull request.", resp.Error)
		assert.Equal(t, before, gitMainOnDisk(t, repo))
		assert.Equal(t, "mergeable", pullGet(t, unlistedClient, repo, index).Pull.State)

		out, ok := protectedBranchPush(t, repo, unlisted)
		assert.False(t, ok, "push succeeded: %s", out)
		assert.Contains(t, out, "is protected and you are not in the push allowlist")
		assert.Equal(t, before, gitMainOnDisk(t, repo))
	})

	t.Run("listed writer", func(t *testing.T) {
		index := newPull(t)
		v := pullWaitMergeable(t, listedClient, repo, index)
		assert.True(t, v.Pull.ViewerCanMerge)
		assert.False(t, v.Pull.ViewerNotInAllowlist)

		status, body := protectedBranchMerge(t, listedClient, repo, index)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		assert.Equal(t, "merged", pullGet(t, listedClient, repo, index).Pull.State)

		// The allowlist lets its people push past the pull request requirement.
		out, ok := protectedBranchPush(t, repo, listed)
		assert.True(t, ok, "push failed: %s", out)
	})

	t.Run("team membership is checked when pushing or merging", func(t *testing.T) {
		index := newPull(t)
		assert.False(t, pullWaitMergeable(t, joinerClient, repo, index).Pull.ViewerCanMerge)

		// Joining and leaving the team does not save the branch protection.
		require.NoError(t, release.AddMember(joiner.ID))
		v := pullWaitMergeable(t, joinerClient, repo, index)
		assert.True(t, v.Pull.ViewerCanMerge)
		assert.False(t, v.Pull.ViewerNotInAllowlist)
		status, body := protectedBranchMerge(t, joinerClient, repo, index)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		out, ok := protectedBranchPush(t, repo, joiner)
		assert.True(t, ok, "push after joining failed: %s", out)

		require.NoError(t, release.RemoveMember(joiner.ID))
		index = newPull(t)
		v = pullWaitMergeable(t, joinerClient, repo, index)
		assert.False(t, v.Pull.ViewerCanMerge)
		assert.True(t, v.Pull.ViewerNotInAllowlist)
		status, body = protectedBranchMerge(t, joinerClient, repo, index)
		assert.Equal(t, http.StatusForbidden, status, "response: %s", body)
		out, ok = protectedBranchPush(t, repo, joiner)
		assert.False(t, ok, "push after leaving succeeded: %s", out)
	})

	t.Run("allowlist turned off", func(t *testing.T) {
		protect(t, map[string]any{"protected": true, "requirePullRequest": true, "enableAllowlist": false})
		index := newPull(t)
		v := pullWaitMergeable(t, unlistedClient, repo, index)
		assert.True(t, v.Pull.ViewerCanMerge)
		assert.False(t, v.Pull.ViewerNotInAllowlist)
		status, body := protectedBranchMerge(t, unlistedClient, repo, index)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)

		out, ok := protectedBranchPush(t, repo, unlisted)
		assert.False(t, ok, "push succeeded: %s", out)
		assert.Contains(t, out, "commits must be merged through pull request")
	})
}
