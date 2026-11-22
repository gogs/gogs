package integrationtests

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/database"
)

// protectedBranchTeam creates a team of the organization with the access to
// the repository and the members.
func protectedBranchTeam(t *testing.T, org *database.User, repo *Repository, mode database.AccessMode, memberIDs ...int64) *database.Team {
	t.Helper()

	team := &database.Team{OrgID: org.ID, Name: UniqueName("team-"), Authorize: mode}
	require.NoError(t, database.Handle.Teams().Create(t.Context(), team))
	r, err := getRepository(t, repo.ID)
	require.NoError(t, err)
	require.NoError(t, database.Handle.Teams().AddRepo(t.Context(), team, r))
	for _, id := range memberIDs {
		require.NoError(t, database.Handle.Teams().AddMember(t.Context(), team.OrgID, team.ID, id))
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
	t.Parallel()

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

	ownerClient := authenticatedClient(t, owner)
	listedClient := authenticatedClient(t, listed)
	unlistedClient := authenticatedClient(t, unlisted)
	joinerClient := authenticatedClient(t, joiner)

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
		require.NoError(t, database.Handle.Teams().AddMember(t.Context(), release.OrgID, release.ID, joiner.ID))
		v := pullWaitMergeable(t, joinerClient, repo, index)
		assert.True(t, v.Pull.ViewerCanMerge)
		assert.False(t, v.Pull.ViewerNotInAllowlist)
		status, body := protectedBranchMerge(t, joinerClient, repo, index)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		out, ok := protectedBranchPush(t, repo, joiner)
		assert.True(t, ok, "push after joining failed: %s", out)

		require.NoError(t, database.Handle.Teams().RemoveMember(t.Context(), release.OrgID, release.ID, joiner.ID))
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

func TestProtectedBranchRules(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	listed := seedUser(t, CreateUserOptions{})
	unlisted := seedUser(t, CreateUserOptions{})
	org := orgSeed(t, owner.ID)
	repo := seedRepository(t, org.ID, CreateRepositoryOptions{AutoInit: true})
	protectedBranchTeam(t, org, repo, database.AccessModeWrite, listed.ID, unlisted.ID)

	ownerClient := authenticatedClient(t, owner)
	protect := func(t *testing.T, branch string, body map[string]any) {
		t.Helper()
		status, resp := ownerClient.do(http.MethodPut, issueAPIPath(repo, "settings", "branches", branch), body)
		require.Equal(t, http.StatusNoContent, status, "response: %s", resp)
	}
	protect(t, "main", map[string]any{
		"protected":          true,
		"requirePullRequest": true,
		"enableAllowlist":    true,
		"allowlistUserIDs":   []int64{listed.ID},
	})

	t.Run("web editor and contents API follow the push rules", func(t *testing.T) {
		editor := func(u *User) string {
			var resp struct {
				Editor string `json:"editor"`
			}
			authenticatedClient(t, u).doJSON(http.MethodGet, issueAPIPath(repo, "src", "main"), nil, http.StatusOK, &resp)
			return resp.Editor
		}
		// People in the allowlist push past the pull request requirement, so
		// they can also edit in the browser.
		assert.Equal(t, "ok", editor(listed))
		assert.Empty(t, editor(unlisted))

		before := gitMainOnDisk(t, repo)
		status, body := apiV1Client(t, unlisted).do(http.MethodPut, apiV1RepoPath(repo, "/contents/unlisted.txt"), map[string]any{
			"message": "Add unlisted.txt",
			"content": "dW5saXN0ZWQK",
			"branch":  "main",
		})
		assert.Equal(t, http.StatusForbidden, status, "response: %s", body)
		assert.Contains(t, string(body), "you are not in the push allowlist")
		assert.Equal(t, before, gitMainOnDisk(t, repo))

		apiV1PutFile(t, apiV1Client(t, listed), repo, "listed.txt", "listed\n", "main")
	})

	t.Run("writer outside the allowlist commits to a new branch in the editor", func(t *testing.T) {
		dir := gitClone(t, gitRemoteURL(t, repo, listed.Name, listed.Password))
		gitRun(t, dir, "push", "origin", "main:refs/heads/dev")
		protect(t, "dev", map[string]any{
			"protected":        true,
			"enableAllowlist":  true,
			"allowlistUserIDs": []int64{listed.ID},
		})
		c := authenticatedClient(t, unlisted)
		var src struct {
			Editor string `json:"editor"`
		}
		c.doJSON(http.MethodGet, issueAPIPath(repo, "src", "dev"), nil, http.StatusOK, &src)
		assert.Equal(t, "ok", src.Editor)
		// The editor says up front that only a new branch can take the commit.
		var target struct {
			DirectCommitRefused string `json:"directCommitRefused"`
		}
		c.doJSON(http.MethodGet, issueAPIPath(repo, "editor", "dev"), nil, http.StatusOK, &target)
		assert.Equal(t, "Branch <code>{branch}</code> is protected and you are not in the push allowlist.", target.DirectCommitRefused)

		commit := func(newBranch string) (int, []byte) {
			return c.do(http.MethodPost, issueAPIPath(repo, "editor", "files"), map[string]any{
				"branch":    "dev",
				"newBranch": newBranch,
				"path":      UniqueName("file-") + ".txt",
				"content":   "content\n",
			})
		}
		before := gitRun(t, t.TempDir(), "--git-dir", database.RepoPath(repo.Owner, repo.Name), "rev-parse", "refs/heads/dev")
		status, body := commit("")
		assert.Equal(t, http.StatusForbidden, status, "response: %s", body)
		var refused struct {
			Fields map[string]string `json:"fields"`
			Branch string            `json:"branch"`
		}
		require.NoError(t, json.Unmarshal(body, &refused))
		assert.Equal(t, map[string]string{"branch": "Branch <code>{branch}</code> is protected and you are not in the push allowlist."}, refused.Fields)
		assert.Equal(t, "dev", refused.Branch)
		assert.Equal(t, before, gitRun(t, t.TempDir(), "--git-dir", database.RepoPath(repo.Owner, repo.Name), "rev-parse", "refs/heads/dev"))

		status, body = commit(UniqueName("proposal-"))
		assert.Equal(t, http.StatusOK, status, "response: %s", body)

		// A new branch with the name of a deleted branch that is still protected.
		gitRun(t, dir, "push", "origin", "main:refs/heads/staging")
		protect(t, "staging", map[string]any{
			"protected":        true,
			"enableAllowlist":  true,
			"allowlistUserIDs": []int64{listed.ID},
		})
		gitRun(t, t.TempDir(), "--git-dir", database.RepoPath(repo.Owner, repo.Name), "branch", "-D", "staging")
		status, body = commit("staging")
		assert.Equal(t, http.StatusForbidden, status, "response: %s", body)
		refused.Fields, refused.Branch = nil, ""
		require.NoError(t, json.Unmarshal(body, &refused))
		assert.Equal(t, map[string]string{"newBranch": "Branch <code>{branch}</code> is protected and you are not in the push allowlist."}, refused.Fields)
		assert.Equal(t, "staging", refused.Branch)
	})

	t.Run("restoring a deleted protected head branch", func(t *testing.T) {
		branch := UniqueName("feature-")
		dir := gitClone(t, gitRemoteURL(t, repo, listed.Name, listed.Password))
		gitRun(t, dir, "checkout", "-b", branch)
		gitCommitFile(t, dir, branch+".txt")
		gitRun(t, dir, "push", "origin", branch)
		listedClient := authenticatedClient(t, listed)
		index := pullCreate(t, listedClient, repo, branch, "Change "+branch)
		pullWaitMergeable(t, listedClient, repo, index)
		status, body := protectedBranchMerge(t, listedClient, repo, index)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)

		protect(t, branch, map[string]any{
			"protected":        true,
			"enableAllowlist":  true,
			"allowlistUserIDs": []int64{listed.ID},
		})
		gitRun(t, t.TempDir(), "--git-dir", database.RepoPath(repo.Owner, repo.Name), "branch", "-D", branch)

		unlistedClient := authenticatedClient(t, unlisted)
		assert.False(t, pullGet(t, unlistedClient, repo, index).Pull.ViewerCanRestoreBranch)
		path := issueAPIPath(repo, "pulls", index, "branch")
		status, body = unlistedClient.do(http.MethodPost, path, map[string]any{})
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
		_, err := gitCommand(t.TempDir(), "--git-dir", database.RepoPath(repo.Owner, repo.Name), "rev-parse", "--verify", "refs/heads/"+branch).CombinedOutput()
		assert.Error(t, err, "branch was restored")

		assert.True(t, pullGet(t, listedClient, repo, index).Pull.ViewerCanRestoreBranch)
		status, body = listedClient.do(http.MethodPost, path, map[string]any{})
		assert.Equal(t, http.StatusNoContent, status, "response: %s", body)
	})

	t.Run("tag with the name of a protected branch", func(t *testing.T) {
		dir := gitClone(t, gitRemoteURL(t, repo, unlisted.Name, unlisted.Password))
		gitRun(t, dir, "tag", "main")
		out, err := gitCommand(dir, "push", "origin", "refs/tags/main").CombinedOutput()
		require.NoError(t, err, "push tag: %s", out)

		out, err = gitCommand(dir, "push", "origin", ":refs/tags/main").CombinedOutput()
		require.NoError(t, err, "delete tag: %s", out)

		// The branch stays protected.
		pushOut, ok := protectedBranchPush(t, repo, unlisted)
		assert.False(t, ok, "push succeeded: %s", pushOut)
		assert.Contains(t, pushOut, "you are not in the push allowlist")
	})

	t.Run("branch created again under its protection", func(t *testing.T) {
		dir := gitClone(t, gitRemoteURL(t, repo, listed.Name, listed.Password))
		gitRun(t, dir, "push", "origin", "main:refs/heads/release")
		protect(t, "release", map[string]any{"protected": true})
		// Protection outlives a branch deleted outside of Gogs.
		gitRun(t, t.TempDir(), "--git-dir", database.RepoPath(repo.Owner, repo.Name), "branch", "-D", "release")

		out, err := gitCommand(dir, "push", "origin", "main:refs/heads/release").CombinedOutput()
		require.NoError(t, err, "push: %s", out)

		out, err = gitCommand(dir, "push", "origin", ":refs/heads/release").CombinedOutput()
		assert.Error(t, err, "delete succeeded: %s", out)
		assert.Contains(t, string(out), "is protected from deletion")
	})

	t.Run("API creates branches and tags under protected names", func(t *testing.T) {
		dir := gitClone(t, gitRemoteURL(t, repo, listed.Name, listed.Password))
		gitRun(t, dir, "push", "origin", "main:refs/heads/hotfix")
		protect(t, "hotfix", map[string]any{
			"protected":        true,
			"enableAllowlist":  true,
			"allowlistUserIDs": []int64{listed.ID},
		})
		gitRun(t, t.TempDir(), "--git-dir", database.RepoPath(repo.Owner, repo.Name), "branch", "-D", "hotfix")
		create := func(u *User, kind, name string) (int, []byte) {
			return apiV1Client(t, u).do(http.MethodPost, apiV1RepoPath(repo, "/"+kind), map[string]any{"name": name})
		}

		status, body := create(unlisted, "tags", "hotfix")
		assert.Equal(t, http.StatusCreated, status, "create tag: %s", body)

		status, body = create(unlisted, "branches", "hotfix")
		assert.Equal(t, http.StatusForbidden, status, "response: %s", body)
		assert.Contains(t, string(body), "you are not in the push allowlist")
		_, err := gitCommand(t.TempDir(), "--git-dir", database.RepoPath(repo.Owner, repo.Name), "rev-parse", "--verify", "refs/heads/hotfix").CombinedOutput()
		assert.Error(t, err, "branch was created")

		status, body = create(listed, "branches", "hotfix")
		assert.Equal(t, http.StatusCreated, status, "response: %s", body)

		// A taken name is reported as such, not as protected.
		status, body = create(unlisted, "branches", "hotfix")
		assert.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", body)
	})
}

func TestProtectedBranchTransferToUser(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	listed := seedUser(t, CreateUserOptions{})
	org := orgSeed(t, owner.ID)
	repo := seedRepository(t, org.ID, CreateRepositoryOptions{AutoInit: true})
	protectedBranchTeam(t, org, repo, database.AccessModeWrite, listed.ID)

	ownerClient := authenticatedClient(t, owner)
	status, body := ownerClient.do(http.MethodPut, issueAPIPath(repo, "settings", "branches", "main"), map[string]any{
		"protected":        true,
		"enableAllowlist":  true,
		"allowlistUserIDs": []int64{listed.ID},
	})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	out, ok := protectedBranchPush(t, repo, owner)
	require.False(t, ok, "push before transfer succeeded: %s", out)

	status, body = ownerClient.do(http.MethodPost, issueAPIPath(repo, "settings", "transfer"), map[string]any{"repoName": repo.Name, "newOwner": owner.Name})
	require.Equal(t, http.StatusOK, status, "response: %s", body)
	moved := *repo
	moved.Owner = owner.Name

	// Settings of user repositories have no allowlist, so one left enabled
	// would keep out everyone not listed by name, e.g., the new owner.
	out, ok = protectedBranchPush(t, &moved, owner)
	assert.True(t, ok, "push after transfer failed: %s", out)

	var got struct {
		Protected       bool `json:"protected"`
		EnableAllowlist bool `json:"enableAllowlist"`
	}
	authenticatedClient(t, owner).doJSON(http.MethodGet, issueAPIPath(&moved, "settings", "branches", "main"), nil, http.StatusOK, &got)
	assert.True(t, got.Protected)
	assert.False(t, got.EnableAllowlist)
}

func TestProtectedBranchReviews(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	author := seedUser(t, CreateUserOptions{})
	reviewer := seedUser(t, CreateUserOptions{})
	org := orgSeed(t, owner.ID)
	repo := seedRepository(t, org.ID, CreateRepositoryOptions{AutoInit: true})
	writers := protectedBranchTeam(t, org, repo, database.AccessModeWrite, author.ID, reviewer.ID)

	ownerClient := authenticatedClient(t, owner)
	protect := func(body map[string]any) {
		t.Helper()
		status, resp := ownerClient.do(http.MethodPut, issueAPIPath(repo, "settings", "branches", "main"), body)
		require.Equal(t, http.StatusNoContent, status, "response: %s", resp)
	}
	var settings struct {
		RequiredApprovals int `json:"requiredApprovals"`
	}

	// Without requiring pull requests, writers would push past the reviews, so
	// they are not kept.
	protect(map[string]any{"protected": true, "requiredApprovals": 1})
	ownerClient.doJSON(http.MethodGet, issueAPIPath(repo, "settings", "branches", "main"), nil, http.StatusOK, &settings)
	assert.Zero(t, settings.RequiredApprovals)
	pushOut, ok := protectedBranchPush(t, repo, author)
	require.True(t, ok, "push failed: %s", pushOut)

	protect(map[string]any{"protected": true, "requirePullRequest": true, "requiredApprovals": 1, "dismissStaleApprovals": true})
	ownerClient.doJSON(http.MethodGet, issueAPIPath(repo, "settings", "branches", "main"), nil, http.StatusOK, &settings)
	require.Equal(t, 1, settings.RequiredApprovals)

	branch := UniqueName("feature-")
	dir := gitClone(t, gitRemoteURL(t, repo, author.Name, author.Password))
	gitRun(t, dir, "checkout", "-b", branch)
	gitCommitFile(t, dir, branch+".txt")
	gitRun(t, dir, "push", "origin", branch)
	authorClient := authenticatedClient(t, author)
	index := pullCreate(t, authorClient, repo, branch, "Change "+branch)
	pullWaitMergeable(t, authorClient, repo, index)

	type reviewView struct {
		MergeBlockers []string `json:"mergeBlockers"`
		Pull          struct {
			ViewerCanMerge bool `json:"viewerCanMerge"`
			Diff           struct {
				Head string `json:"head"`
			} `json:"diff"`
		} `json:"pull"`
	}
	view := func(c *client) reviewView {
		t.Helper()
		var v reviewView
		c.doJSON(http.MethodGet, issueAPIPath(repo, "pulls", index), nil, http.StatusOK, &v)
		return v
	}
	reviewerClient := authenticatedClient(t, reviewer)
	approve := func() {
		t.Helper()
		status, body := reviewerClient.do(http.MethodPost, issueAPIPath(repo, "pulls", index, "reviews"), map[string]any{
			"state":   "approve",
			"headSHA": view(reviewerClient).Pull.Diff.Head,
		})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	}

	// Admins are held to the reviews too.
	v := view(ownerClient)
	assert.Equal(t, []string{"approvals"}, v.MergeBlockers)
	assert.False(t, v.Pull.ViewerCanMerge)
	status, body := protectedBranchMerge(t, ownerClient, repo, index)
	assert.Equal(t, http.StatusConflict, status, "response: %s", body)

	approve()
	v = view(ownerClient)
	assert.Empty(t, v.MergeBlockers)
	assert.True(t, v.Pull.ViewerCanMerge)

	// New commits dismiss the approval of the older head.
	gitCommitFile(t, dir, UniqueName("more-")+".txt")
	gitRun(t, dir, "push", "origin", branch)
	require.Eventually(t, func() bool {
		return len(view(ownerClient).MergeBlockers) > 0
	}, 30*time.Second, 200*time.Millisecond)
	status, body = protectedBranchMerge(t, ownerClient, repo, index)
	assert.Equal(t, http.StatusConflict, status, "response: %s", body)

	approve()
	assert.Empty(t, view(ownerClient).MergeBlockers)

	// Approvals count only while their reviewers can still write.
	require.NoError(t, database.Handle.Teams().RemoveMember(t.Context(), org.ID, writers.ID, reviewer.ID))
	assert.Equal(t, []string{"approvals"}, view(ownerClient).MergeBlockers)
	status, body = protectedBranchMerge(t, ownerClient, repo, index)
	assert.Equal(t, http.StatusConflict, status, "response: %s", body)
	require.NoError(t, database.Handle.Teams().AddMember(t.Context(), org.ID, writers.ID, reviewer.ID))
	assert.Empty(t, view(ownerClient).MergeBlockers)

	// Without dismissing stale approvals, an approval of an older head counts.
	protect(map[string]any{"protected": true, "requirePullRequest": true, "requiredApprovals": 1})
	newHead := gitCommitFile(t, dir, UniqueName("more-")+".txt")
	gitRun(t, dir, "push", "origin", branch)
	require.Eventually(t, func() bool {
		return view(ownerClient).Pull.Diff.Head == newHead
	}, 30*time.Second, 200*time.Millisecond)
	assert.Empty(t, view(ownerClient).MergeBlockers)

	status, body = protectedBranchMerge(t, ownerClient, repo, index)
	assert.Equal(t, http.StatusNoContent, status, "response: %s", body)
}
