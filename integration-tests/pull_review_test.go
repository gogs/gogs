package integrationtests

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/database"
)

func TestPullReviews(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	author := seedUser(t, CreateUserOptions{})
	reviewer := seedUser(t, CreateUserOptions{})
	reader := seedUser(t, CreateUserOptions{})
	org := orgSeed(t, owner.ID)
	repo := seedRepository(t, org.ID, CreateRepositoryOptions{AutoInit: true})
	protectedBranchTeam(t, org, repo, database.AccessModeWrite, author.ID, reviewer.ID)
	protectedBranchTeam(t, org, repo, database.AccessModeRead, reader.ID)

	ownerClient := authenticatedClient(t, owner)
	status, body := ownerClient.do(http.MethodPut, issueAPIPath(repo, "settings", "branches", "main"), map[string]any{
		"protected":                    true,
		"requirePullRequest":           true,
		"requiredApprovals":            1,
		"requireResolvedConversations": true,
	})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)

	branch := UniqueName("feature-")
	file := branch + ".txt"
	dir := gitClone(t, gitRemoteURL(t, repo, author.Name, author.Password))
	gitRun(t, dir, "checkout", "-b", branch)
	gitCommitFile(t, dir, file)
	gitRun(t, dir, "push", "origin", branch)
	authorClient := authenticatedClient(t, author)
	index := pullCreate(t, authorClient, repo, branch, "Change "+branch)
	pullWaitMergeable(t, authorClient, repo, index)

	type reviewView struct {
		Reviewers []struct {
			User struct {
				Name string `json:"name"`
			} `json:"user"`
			State string `json:"state"`
		} `json:"reviewers"`
		MergeBlockers       []string        `json:"mergeBlockers"`
		ViewerPendingReview *map[string]any `json:"viewerPendingReview"`
		Threads             []struct {
			ID int64 `json:"id"`
		} `json:"threads"`
		Pull struct {
			Diff struct {
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
	head := view(authorClient).Pull.Diff.Head
	path := func(parts ...any) string { return issueAPIPath(repo, append([]any{"pulls", index}, parts...)...) }

	t.Run("review requests", func(t *testing.T) {
		status, body := authorClient.do(http.MethodPut, path("reviewers", reviewer.Name), map[string]any{})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		v := view(authorClient)
		require.Len(t, v.Reviewers, 1)
		assert.Equal(t, reviewer.Name, v.Reviewers[0].User.Name)
		assert.Equal(t, "requested", v.Reviewers[0].State)

		// Reviews count only from writers, so none are requested from others.
		status, body = authorClient.do(http.MethodPut, path("reviewers", reader.Name), map[string]any{})
		assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)

		// Only writers and the author ask for reviews.
		status, body = authenticatedClient(t, reader).do(http.MethodPut, path("reviewers", reviewer.Name), map[string]any{})
		assert.Equal(t, http.StatusForbidden, status, "response: %s", body)

		status, body = authorClient.do(http.MethodDelete, path("reviewers", reviewer.Name), map[string]any{})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		assert.Empty(t, view(authorClient).Reviewers)
	})

	reviewerClient := authenticatedClient(t, reviewer)
	codeComment := func(pending bool) (int, []byte) {
		return reviewerClient.do(http.MethodPost, path("code-comments"), map[string]any{
			"path":      file,
			"side":      "new",
			"startSide": "new",
			"startLine": 1,
			"line":      1,
			"headSHA":   head,
			"content":   "Why this line?",
			"pending":   pending,
		})
	}

	t.Run("pending review is discarded", func(t *testing.T) {
		status, body := codeComment(true)
		require.Equal(t, http.StatusCreated, status, "response: %s", body)
		require.NotNil(t, view(reviewerClient).ViewerPendingReview)
		// Others do not see a pending comment.
		assert.Empty(t, view(authorClient).Threads)

		status, body = reviewerClient.do(http.MethodDelete, path("reviews", "pending"), map[string]any{})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		v := view(reviewerClient)
		assert.Nil(t, v.ViewerPendingReview)
		assert.Empty(t, v.Threads)
	})

	t.Run("lines outside the diff", func(t *testing.T) {
		status, body := reviewerClient.do(http.MethodPost, path("code-comments"), map[string]any{
			"path": "README.md", "side": "new", "startSide": "new", "startLine": 1, "line": 1,
			"headSHA": head, "content": "Not in the diff",
		})
		assert.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", body)
	})

	t.Run("webhooks send replies but not pending comments", func(t *testing.T) {
		url, deliveries := repoSettingsWebhookReceiver(t)
		repoSettingsCreateWebhookFor(t, ownerClient, repo, url, true, map[string]bool{"issueComment": true})
		type payload struct {
			Action  string `json:"action"`
			Comment struct {
				Body string `json:"body"`
			} `json:"comment"`
		}
		next := func() payload {
			t.Helper()
			var p payload
			require.NoError(t, json.Unmarshal(repoSettingsNextDelivery(t, deliveries).Body, &p))
			return p
		}
		var root, pending struct {
			ID int64 `json:"id"`
		}
		authorClient.doJSON(http.MethodPost, path("comments"), map[string]any{"content": "Root"}, http.StatusCreated, &root)
		got := next()
		assert.Equal(t, "created", got.Action)
		assert.Equal(t, "Root", got.Comment.Body)

		status, body := codeComment(true)
		require.Equal(t, http.StatusCreated, status, "response: %s", body)
		require.NoError(t, json.Unmarshal(body, &pending))
		status, body = reviewerClient.do(http.MethodPatch, path("comments", pending.ID), map[string]any{"content": "Secret draft"})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)

		var reply struct{}
		authorClient.doJSON(http.MethodPost, path("comments"), map[string]any{"content": "Reply", "replyTo": root.ID}, http.StatusCreated, &reply)
		// An edit of the pending comment would have been delivered first.
		got = next()
		assert.Equal(t, "created", got.Action)
		assert.Equal(t, "Reply", got.Comment.Body)

		status, body = reviewerClient.do(http.MethodDelete, path("reviews", "pending"), map[string]any{})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	})

	review := func(c *client, state, headSHA string) (int, []byte) {
		return c.do(http.MethodPost, path("reviews"), map[string]any{"state": state, "headSHA": headSHA, "content": "Looked at it."})
	}

	t.Run("authors cannot approve or request changes on their own pull request", func(t *testing.T) {
		for _, state := range []string{"approve", "request_changes"} {
			status, body := review(authorClient, state, head)
			assert.Equal(t, http.StatusBadRequest, status, "%s response: %s", state, body)
		}
		assert.Empty(t, view(ownerClient).Reviewers)
	})

	t.Run("reviews of an older head are refused", func(t *testing.T) {
		older := gitRun(t, dir, "rev-parse", "HEAD~1")
		status, body := review(reviewerClient, "approve", older)
		assert.Equal(t, http.StatusConflict, status, "response: %s", body)
		assert.Empty(t, view(ownerClient).Reviewers)
	})

	t.Run("only reviews from writers count toward merging", func(t *testing.T) {
		status, body := review(authenticatedClient(t, reader), "approve", head)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		assert.Contains(t, view(ownerClient).MergeBlockers, "approvals")

		status, body = review(reviewerClient, "request_changes", head)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		assert.Contains(t, view(ownerClient).MergeBlockers, "changes_requested")
		status, body = protectedBranchMerge(t, ownerClient, repo, index)
		assert.Equal(t, http.StatusConflict, status, "response: %s", body)
	})

	t.Run("unresolved conversations block the merge", func(t *testing.T) {
		status, body := codeComment(false)
		require.Equal(t, http.StatusCreated, status, "response: %s", body)
		status, body = reviewerClient.do(http.MethodPost, path("reviews"), map[string]any{"state": "approve", "headSHA": head})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)

		v := view(ownerClient)
		require.Len(t, v.Threads, 1)
		assert.Equal(t, []string{"unresolved_conversations"}, v.MergeBlockers)
		thread := path("threads", v.Threads[0].ID, "resolved")

		// Readers cannot resolve threads.
		status, body = authenticatedClient(t, reader).do(http.MethodPut, thread, map[string]any{})
		assert.Equal(t, http.StatusForbidden, status, "response: %s", body)

		status, body = authorClient.do(http.MethodPut, thread, map[string]any{})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		assert.Empty(t, view(ownerClient).MergeBlockers)

		status, body = authorClient.do(http.MethodDelete, thread, map[string]any{})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		assert.Equal(t, []string{"unresolved_conversations"}, view(ownerClient).MergeBlockers)
		status, body = protectedBranchMerge(t, ownerClient, repo, index)
		assert.Equal(t, http.StatusConflict, status, "response: %s", body)

		status, body = reviewerClient.do(http.MethodPut, thread, map[string]any{})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		status, body = protectedBranchMerge(t, ownerClient, repo, index)
		assert.Equal(t, http.StatusNoContent, status, "response: %s", body)
	})

	t.Run("replies to a merged pull request do not wait for a review", func(t *testing.T) {
		v := view(ownerClient)
		require.NotEmpty(t, v.Threads)
		var reply struct {
			ID int64 `json:"id"`
		}
		reviewerClient.doJSON(http.MethodPost, path("comments"), map[string]any{
			"content": "After the merge", "replyTo": v.Threads[0].ID, "pending": true,
		}, http.StatusCreated, &reply)
		assert.Nil(t, view(reviewerClient).ViewerPendingReview)
		comment, err := database.Handle.Comments().GetByID(t.Context(), reply.ID)
		require.NoError(t, err)
		assert.False(t, comment.IsPending)
	})

	t.Run("checking repository stats keeps code comments counted", func(t *testing.T) {
		issue, err := database.Handle.Issues().GetByIndex(t.Context(), repo.ID, index)
		require.NoError(t, err)
		require.Positive(t, issue.NumComments)
		database.CheckRepoStats(logger)
		checked, err := database.Handle.Issues().GetByIndex(t.Context(), repo.ID, index)
		require.NoError(t, err)
		assert.Equal(t, issue.NumComments, checked.NumComments)
	})

	t.Run("deleting the repository deletes its reviews", func(t *testing.T) {
		issue, err := database.Handle.Issues().GetByIndex(t.Context(), repo.ID, index)
		require.NoError(t, err)
		reviews, err := database.Handle.Reviews().ListSubmitted(t.Context(), issue.ID)
		require.NoError(t, err)
		require.NotEmpty(t, reviews)

		status, body := ownerClient.do(http.MethodDelete, repoSettingsAPIPath(repo, ""), map[string]any{"repoName": repo.Name})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		reviews, err = database.Handle.Reviews().ListSubmitted(t.Context(), issue.ID)
		require.NoError(t, err)
		assert.Empty(t, reviews)
	})
}
