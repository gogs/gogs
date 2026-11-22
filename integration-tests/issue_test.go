package integrationtests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/database"
)

type issueLabel struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type issueView struct {
	Index       int64  `json:"index"`
	Title       string `json:"title"`
	IsClosed    bool   `json:"isClosed"`
	Content     string `json:"content"`
	ContentHTML string `json:"contentHTML"`
	Poster      struct {
		Name string `json:"name"`
	} `json:"poster"`
	Comments []struct {
		ID            int64  `json:"id"`
		Type          string `json:"type"`
		Content       string `json:"content"`
		ContentHTML   string `json:"contentHTML"`
		ViewerCanEdit bool   `json:"viewerCanEdit"`
	} `json:"comments"`
	Labels    []issueLabel `json:"labels"`
	Milestone *struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"milestone"`
	Assignee *struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"assignee"`
	LabelOptions   []issueLabel `json:"labelOptions"`
	ViewerCanEdit  bool         `json:"viewerCanEdit"`
	ViewerCanWrite bool         `json:"viewerCanWrite"`
}

func issueGet(t *testing.T, c *client, repo *Repository, index int64) *issueView {
	t.Helper()

	var v issueView
	c.doJSON(http.MethodGet, issueAPIPath(repo, "issues", index), nil, http.StatusOK, &v)
	return &v
}

func issueFromDatabase(t *testing.T, repo *Repository, index int64) *database.Issue {
	t.Helper()

	issue, err := database.Handle.Issues().GetByIndex(t.Context(), repo.ID, index)
	require.NoError(t, err)
	return issue
}

func TestIssueCreateAndView(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	ownerClient := authenticatedClient(t, owner)
	otherClient := authenticatedClient(t, other)
	anonymous := newClient(t)

	t.Run("new issue form", func(t *testing.T) {
		var resp struct {
			BlankIssuesEnabled bool `json:"blankIssuesEnabled"`
			ViewerCanWrite     bool `json:"viewerCanWrite"`
			Attachment         *struct {
				MaxFiles int `json:"maxFiles"`
			} `json:"attachment"`
		}
		ownerClient.doJSON(http.MethodGet, issueAPIPath(repo, "issues", "new"), nil, http.StatusOK, &resp)
		assert.True(t, resp.BlankIssuesEnabled)
		assert.True(t, resp.ViewerCanWrite)
		require.NotNil(t, resp.Attachment)
		assert.Equal(t, 5, resp.Attachment.MaxFiles)

		otherClient.doJSON(http.MethodGet, issueAPIPath(repo, "issues", "new"), nil, http.StatusOK, &resp)
		assert.False(t, resp.ViewerCanWrite)

		status, body := anonymous.do(http.MethodGet, issueAPIPath(repo, "issues", "new"), nil)
		assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
	})

	title := UniqueName("Crash ")
	index := issueCreate(t, otherClient, repo, title, "It **crashes**.")

	got := issueFromDatabase(t, repo, index)
	assert.Equal(t, title, got.Title)
	assert.Equal(t, "It **crashes**.", got.Content)
	assert.Equal(t, other.ID, got.PosterID)
	assert.False(t, got.IsPull)

	t.Run("view", func(t *testing.T) {
		v := issueGet(t, anonymous, repo, index)
		assert.Equal(t, index, v.Index)
		assert.Equal(t, title, v.Title)
		assert.Contains(t, v.ContentHTML, "<strong>crashes</strong>")
		assert.Equal(t, other.Name, v.Poster.Name)
		assert.False(t, v.ViewerCanEdit)
		assert.False(t, v.ViewerCanWrite)

		v = issueGet(t, otherClient, repo, index)
		assert.True(t, v.ViewerCanEdit, "the poster can edit the issue")
		assert.False(t, v.ViewerCanWrite)

		status, body := anonymous.do(http.MethodGet, issueAPIPath(repo, "issues", index+1000), nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})

	t.Run("list", func(t *testing.T) {
		var list struct {
			Items []struct {
				Index int64  `json:"index"`
				Title string `json:"title"`
			} `json:"items"`
			OpenCount   int64 `json:"openCount"`
			ClosedCount int64 `json:"closedCount"`
		}
		anonymous.doJSON(http.MethodGet, issueAPIPath(repo, "issues"), nil, http.StatusOK, &list)
		require.Len(t, list.Items, 1)
		assert.Equal(t, index, list.Items[0].Index)
		assert.Equal(t, title, list.Items[0].Title)
		assert.Equal(t, int64(1), list.OpenCount)
		assert.Equal(t, int64(0), list.ClosedCount)

		anonymous.doJSON(http.MethodGet, issueAPIPath(repo, "issues")+"?state=closed", nil, http.StatusOK, &list)
		assert.Empty(t, list.Items)

		// Lists about the viewer need an authenticated viewer.
		status, body := anonymous.do(http.MethodGet, issueAPIPath(repo, "issues")+"?type=created_by", nil)
		assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
		ownerClient.doJSON(http.MethodGet, issueAPIPath(repo, "issues")+"?type=created_by", nil, http.StatusOK, &list)
		assert.Empty(t, list.Items, "the owner did not open the issue")
		otherClient.doJSON(http.MethodGet, issueAPIPath(repo, "issues")+"?type=created_by", nil, http.StatusOK, &list)
		assert.Len(t, list.Items, 1)
	})

	t.Run("validation and access", func(t *testing.T) {
		status, body := ownerClient.do(http.MethodPost, issueAPIPath(repo, "issues"), map[string]any{"title": "  "})
		assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)
		assert.Contains(t, string(body), `"title"`)

		status, body = anonymous.do(http.MethodPost, issueAPIPath(repo, "issues"), map[string]any{"title": "Anonymous"})
		assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)

		// A label is only set by writers, so the one of another user is
		// dropped rather than applied.
		labelID := issueCreateLabel(t, repo, "only-writers")
		var created struct {
			Index int64 `json:"index"`
		}
		otherClient.doJSON(http.MethodPost, issueAPIPath(repo, "issues"), map[string]any{
			"title":    "With a label",
			"labelIDs": []int64{labelID},
		}, http.StatusCreated, &created)
		labels, err := database.Handle.Labels().ListByIssueID(t.Context(), issueFromDatabase(t, repo, created.Index).ID)
		require.NoError(t, err)
		assert.Empty(t, labels)
	})
}

func TestIssuePrivateRepository(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
	ownerClient := authenticatedClient(t, owner)
	otherClient := authenticatedClient(t, other)

	index := issueCreate(t, ownerClient, repo, "Secret", "")
	assert.Equal(t, "Secret", issueGet(t, ownerClient, repo, index).Title)

	for name, c := range map[string]*client{"anonymous": newClient(t), "other": otherClient} {
		for _, path := range []string{issueAPIPath(repo, "issues"), issueAPIPath(repo, "issues", index)} {
			status, body := c.do(http.MethodGet, path, nil)
			assert.Equal(t, http.StatusNotFound, status, "%s viewer, GET %s, response: %s", name, path, body)
		}
	}
	status, body := otherClient.do(http.MethodPost, issueAPIPath(repo, "issues"), map[string]any{"title": "Intruder"})
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
}

func TestIssueEditAndState(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	poster := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	ownerClient := authenticatedClient(t, owner)
	posterClient := authenticatedClient(t, poster)
	otherClient := authenticatedClient(t, other)

	index := issueCreate(t, posterClient, repo, "Typo", "Old content")
	path := issueAPIPath(repo, "issues", index)

	status, body := posterClient.do(http.MethodPatch, path, map[string]any{"title": "Fixed title", "content": "New _content_"})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	got := issueFromDatabase(t, repo, index)
	assert.Equal(t, "Fixed title", got.Title)
	assert.Equal(t, "New _content_", got.Content)

	status, body = posterClient.do(http.MethodPatch, path, map[string]any{"title": ""})
	assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)
	status, body = otherClient.do(http.MethodPatch, path, map[string]any{"title": "Hijacked"})
	assert.Equal(t, http.StatusForbidden, status, "response: %s", body)
	status, body = newClient(t).do(http.MethodPatch, path, map[string]any{"title": "Hijacked"})
	assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
	assert.Equal(t, "Fixed title", issueFromDatabase(t, repo, index).Title)

	status, body = otherClient.do(http.MethodPut, path+"/closed", map[string]any{})
	assert.Equal(t, http.StatusForbidden, status, "response: %s", body)
	assert.False(t, issueFromDatabase(t, repo, index).IsClosed)

	status, body = ownerClient.do(http.MethodPut, path+"/closed", map[string]any{})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.True(t, issueFromDatabase(t, repo, index).IsClosed)

	status, body = posterClient.do(http.MethodDelete, path+"/closed", map[string]any{})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.False(t, issueFromDatabase(t, repo, index).IsClosed)

	var types []string
	for _, c := range issueGet(t, ownerClient, repo, index).Comments {
		types = append(types, c.Type)
	}
	assert.Equal(t, []string{"close", "reopen"}, types)
}

func TestIssueComments(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	commenter := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	ownerClient := authenticatedClient(t, owner)
	commenterClient := authenticatedClient(t, commenter)
	otherClient := authenticatedClient(t, other)

	index := issueCreate(t, ownerClient, repo, "Discuss", "")
	path := issueAPIPath(repo, "issues", index, "comments")

	var created struct {
		ID int64 `json:"id"`
	}
	commenterClient.doJSON(http.MethodPost, path, map[string]any{"content": "I **agree**."}, http.StatusCreated, &created)
	comment, err := database.Handle.Comments().GetByID(t.Context(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, "I **agree**.", comment.Content)
	assert.Equal(t, commenter.ID, comment.PosterID)
	assert.Equal(t, issueFromDatabase(t, repo, index).ID, comment.IssueID)

	v := issueGet(t, commenterClient, repo, index)
	require.Len(t, v.Comments, 1)
	assert.Equal(t, "comment", v.Comments[0].Type)
	assert.Contains(t, v.Comments[0].ContentHTML, "<strong>agree</strong>")
	assert.True(t, v.Comments[0].ViewerCanEdit)
	assert.False(t, issueGet(t, otherClient, repo, index).Comments[0].ViewerCanEdit)

	status, body := commenterClient.do(http.MethodPost, path, map[string]any{"content": " "})
	assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)
	status, body = newClient(t).do(http.MethodPost, path, map[string]any{"content": "Anonymous"})
	assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)

	commentPath := issueAPIPath(repo, "issues", index, "comments", created.ID)
	// Only the poster and admins of the repository can change a comment.
	status, body = otherClient.do(http.MethodPatch, commentPath, map[string]any{"content": "Hijacked"})
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	status, body = otherClient.do(http.MethodDelete, commentPath, map[string]any{})
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

	status, body = commenterClient.do(http.MethodPatch, commentPath, map[string]any{"content": "I disagree."})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	comment, err = database.Handle.Comments().GetByID(t.Context(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, "I disagree.", comment.Content)

	status, body = ownerClient.do(http.MethodDelete, commentPath, map[string]any{})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	_, err = database.Handle.Comments().GetByID(t.Context(), created.ID)
	assert.True(t, database.IsErrCommentNotExist(err), "comment not deleted: %v", err)
	assert.Empty(t, issueGet(t, ownerClient, repo, index).Comments)
}

func TestIssueLabelsMilestoneAndAssignee(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	otherRepo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	ownerClient := authenticatedClient(t, owner)
	otherClient := authenticatedClient(t, other)

	index := issueCreate(t, otherClient, repo, "Triage me", "")
	path := issueAPIPath(repo, "issues", index)
	bug := issueCreateLabel(t, repo, "bug")
	feature := issueCreateLabel(t, repo, "feature")
	foreign := issueCreateLabel(t, otherRepo, "foreign")

	labelNames := func() []string {
		var names []string
		for _, l := range issueGet(t, ownerClient, repo, index).Labels {
			names = append(names, l.Name)
		}
		slices.Sort(names)
		return names
	}

	t.Run("labels", func(t *testing.T) {
		status, body := ownerClient.do(http.MethodPut, path+"/labels", map[string]any{"ids": []int64{bug, feature}})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		assert.Equal(t, []string{"bug", "feature"}, labelNames())

		status, body = ownerClient.do(http.MethodPut, path+"/labels", map[string]any{"ids": []int64{feature}})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		assert.Equal(t, []string{"feature"}, labelNames())

		status, body = ownerClient.do(http.MethodPut, path+"/labels", map[string]any{"ids": []int64{foreign}})
		assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)
		status, body = otherClient.do(http.MethodPut, path+"/labels", map[string]any{"ids": []int64{bug}})
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
		assert.Equal(t, []string{"feature"}, labelNames())
	})

	t.Run("milestone", func(t *testing.T) {
		milestoneID := issueCreateMilestone(t, repo, "v1.0")
		foreignMilestoneID := issueCreateMilestone(t, otherRepo, "v9.0")

		status, body := ownerClient.do(http.MethodPut, path+"/milestone", map[string]any{"id": milestoneID})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		assert.Equal(t, milestoneID, issueFromDatabase(t, repo, index).MilestoneID)
		v := issueGet(t, ownerClient, repo, index)
		require.NotNil(t, v.Milestone)
		assert.Equal(t, "v1.0", v.Milestone.Name)
		m, err := database.Handle.Milestones().GetByID(t.Context(), repo.ID, milestoneID)
		require.NoError(t, err)
		assert.Equal(t, 1, m.NumIssues)

		status, body = ownerClient.do(http.MethodPut, path+"/milestone", map[string]any{"id": foreignMilestoneID})
		assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)
		status, body = otherClient.do(http.MethodPut, path+"/milestone", map[string]any{"id": 0})
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
		assert.Equal(t, milestoneID, issueFromDatabase(t, repo, index).MilestoneID)

		status, body = ownerClient.do(http.MethodPut, path+"/milestone", map[string]any{"id": 0})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		assert.Zero(t, issueFromDatabase(t, repo, index).MilestoneID)
	})

	t.Run("assignee", func(t *testing.T) {
		// Issues are only assigned to those who can read the repository, and
		// anyone can read a public one.
		outsider := seedUser(t, CreateUserOptions{})
		privateRepo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
		privateIndex := issueCreate(t, ownerClient, privateRepo, "Private", "")
		status, body := ownerClient.do(http.MethodPut, issueAPIPath(privateRepo, "issues", privateIndex, "assignee"), map[string]any{"id": outsider.ID})
		assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)
		assert.Zero(t, issueFromDatabase(t, privateRepo, privateIndex).AssigneeID)

		status, body = ownerClient.do(http.MethodPut, path+"/assignee", map[string]any{"id": owner.ID})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		assert.Equal(t, owner.ID, issueFromDatabase(t, repo, index).AssigneeID)
		v := issueGet(t, ownerClient, repo, index)
		require.NotNil(t, v.Assignee)
		assert.Equal(t, owner.Name, v.Assignee.Name)

		status, body = otherClient.do(http.MethodPut, path+"/assignee", map[string]any{"id": other.ID})
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
		assert.Equal(t, owner.ID, issueFromDatabase(t, repo, index).AssigneeID)

		status, body = ownerClient.do(http.MethodPut, path+"/assignee", map[string]any{"id": 0})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		assert.Zero(t, issueFromDatabase(t, repo, index).AssigneeID)
	})
}

func TestIssueCreateConcurrently(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	c := authenticatedClient(t, owner)

	create := lockedRequest{http.MethodPost, issueAPIPath(repo, "issues"), map[string]any{"title": "Same time"}}
	statuses := c.doBothWhileLocked("SELECT id FROM repository WHERE id = ? FOR UPDATE", repo.ID, create, create)
	assert.Equal(t, []int{http.StatusCreated, http.StatusCreated}, statuses)
	assert.Equal(t, "Same time", issueFromDatabase(t, repo, 1).Title)
	assert.Equal(t, "Same time", issueFromDatabase(t, repo, 2).Title)
	assert.Equal(t, int64(3), issueCreate(t, c, repo, "Next", ""))

	t.Run("while closing another issue", func(t *testing.T) {
		// Closing an issue locks its labels by name (not by ID, as here), then
		// its milestone, then the repository. Creating an issue must lock them
		// in the same order.
		zeta := issueCreateLabel(t, repo, "zeta")
		alpha := issueCreateLabel(t, repo, "alpha")
		milestoneID := issueCreateMilestone(t, repo, "v1")
		labeled := map[string]any{"title": "Labeled", "labelIDs": []int64{zeta, alpha}, "milestoneID": milestoneID}
		for _, locked := range []struct {
			table string
			id    int64
		}{{"label", alpha}, {"milestone", milestoneID}} {
			var created struct {
				Index int64 `json:"index"`
			}
			c.doJSON(http.MethodPost, issueAPIPath(repo, "issues"), labeled, http.StatusCreated, &created)
			// On PostgreSQL, updating a row moves it to the end of the table,
			// so the row order differs from the name order too.
			label, err := database.Handle.Labels().GetByID(t.Context(), repo.ID, alpha)
			require.NoError(t, err)
			require.NoError(t, database.Handle.Labels().Update(t.Context(), label))

			statuses := c.doBothWhileLocked("SELECT id FROM "+locked.table+" WHERE id = ? FOR UPDATE", locked.id,
				lockedRequest{http.MethodPut, issueAPIPath(repo, "issues", created.Index, "closed"), map[string]any{}},
				lockedRequest{http.MethodPost, issueAPIPath(repo, "issues"), labeled})
			assert.Equal(t, []int{http.StatusNoContent, http.StatusCreated}, statuses, "while %s is locked", locked.table)
		}
	})
}

// Watchers of a repository see its issue activity in their feeds, and other
// users do not.
func TestIssueActivityReachesWatchers(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	watcher := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	ownerClient := authenticatedClient(t, owner)
	watcherClient := authenticatedClient(t, watcher)
	status, body := watcherClient.do(http.MethodPost, repoSettingsAPIPath(repo, "/watch"), repoSettingsNoBody)
	require.Equal(t, http.StatusOK, status, "response: %s", body)

	index := issueCreate(t, ownerClient, repo, "Watched", "")
	status, body = ownerClient.do(http.MethodPost, issueAPIPath(repo, "issues", index, "comments"), map[string]any{"content": "A comment"})
	require.Equal(t, http.StatusCreated, status, "response: %s", body)
	status, body = ownerClient.do(http.MethodPut, issueAPIPath(repo, "issues", index, "closed"), map[string]any{})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	status, body = ownerClient.do(http.MethodDelete, issueAPIPath(repo, "issues", index, "closed"), map[string]any{})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)

	issueOps := func(c *client) []database.ActionType {
		t.Helper()
		ops := []database.ActionType{}
		for _, item := range orgActivity(t, c, "/api/web/dashboard/activity") {
			op := database.ActionType(item.OpType)
			if item.RepoURL == "/"+repo.Owner+"/"+repo.Name && op != database.ActionCreateRepo {
				ops = append(ops, op)
			}
		}
		slices.Sort(ops)
		return ops
	}
	want := []database.ActionType{database.ActionCreateIssue, database.ActionCommentIssue, database.ActionCloseIssue, database.ActionReopenIssue}
	assert.Equal(t, want, issueOps(watcherClient))
	assert.Equal(t, want, issueOps(ownerClient))
	assert.Empty(t, issueOps(authenticatedClient(t, other)))
}

func TestIssueChangeConcurrently(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	c := authenticatedClient(t, owner)
	create := func(milestoneID int64) (index, id int64) {
		t.Helper()
		var created struct {
			Index int64 `json:"index"`
		}
		c.doJSON(http.MethodPost, issueAPIPath(repo, "issues"), map[string]any{"title": "Issue", "milestoneID": milestoneID}, http.StatusCreated, &created)
		return created.Index, issueFromDatabase(t, repo, created.Index).ID
	}
	moveTo := func(index, milestoneID int64) lockedRequest {
		return lockedRequest{http.MethodPut, issueAPIPath(repo, "issues", index, "milestone"), map[string]any{"id": milestoneID}}
	}
	closeIssue := func(index int64) lockedRequest {
		return lockedRequest{http.MethodPut, issueAPIPath(repo, "issues", index, "closed"), map[string]any{}}
	}
	counts := func(milestoneID int64) []int {
		t.Helper()
		m, err := database.Handle.Milestones().GetByID(t.Context(), repo.ID, milestoneID)
		require.NoError(t, err)
		return []int{m.NumIssues, m.NumClosedIssues}
	}

	t.Run("moving issues between milestones both ways", func(t *testing.T) {
		m1 := issueCreateMilestone(t, repo, "both-ways-1")
		m2 := issueCreateMilestone(t, repo, "both-ways-2")
		y, _ := create(m1)
		v, _ := create(m2)

		statuses := c.doBothWhileLocked("SELECT id FROM milestone WHERE id = ? FOR UPDATE", m1, moveTo(y, m2), moveTo(v, m1))
		assert.Equal(t, []int{http.StatusNoContent, http.StatusNoContent}, statuses)
		assert.Equal(t, m2, issueFromDatabase(t, repo, y).MilestoneID)
		assert.Equal(t, m1, issueFromDatabase(t, repo, v).MilestoneID)
		assert.Equal(t, []int{1, 0}, counts(m1))
		assert.Equal(t, []int{1, 0}, counts(m2))
	})

	t.Run("closing an issue while moving it to another milestone", func(t *testing.T) {
		m1 := issueCreateMilestone(t, repo, "closing-1")
		m2 := issueCreateMilestone(t, repo, "closing-2")
		index, _ := create(m1)

		statuses := c.doBothWhileLocked("SELECT id FROM milestone WHERE id = ? FOR UPDATE", m1, moveTo(index, m2), closeIssue(index))
		assert.Equal(t, []int{http.StatusNoContent, http.StatusNoContent}, statuses)
		issue := issueFromDatabase(t, repo, index)
		assert.True(t, issue.IsClosed)
		assert.Equal(t, m2, issue.MilestoneID)
		assert.Equal(t, []int{0, 0}, counts(m1))
		assert.Equal(t, []int{1, 1}, counts(m2))
	})

	t.Run("moving an issue that was closed meanwhile", func(t *testing.T) {
		// The move loads the issue while it is open and saves its milestone
		// after the close.
		m := issueCreateMilestone(t, repo, "closed-meanwhile")
		index, id := create(0)

		statuses := c.doBothWhileLocked("SELECT id FROM issue WHERE id = ? FOR UPDATE", id, closeIssue(index), moveTo(index, m))
		assert.Equal(t, []int{http.StatusNoContent, http.StatusNoContent}, statuses)
		issue := issueFromDatabase(t, repo, index)
		assert.True(t, issue.IsClosed)
		assert.Equal(t, m, issue.MilestoneID)
		assert.Equal(t, []int{1, 1}, counts(m))
	})

	t.Run("changing the assignee while closing the issue", func(t *testing.T) {
		index, _ := create(0)

		statuses := c.doBothWhileLocked("SELECT id FROM repository WHERE id = ? FOR UPDATE", repo.ID, closeIssue(index),
			lockedRequest{http.MethodPut, issueAPIPath(repo, "issues", index, "assignee"), map[string]any{"id": owner.ID}})
		assert.Equal(t, []int{http.StatusNoContent, http.StatusNoContent}, statuses)
		issue := issueFromDatabase(t, repo, index)
		assert.True(t, issue.IsClosed)
		assert.Equal(t, owner.ID, issue.AssigneeID)
	})

	t.Run("deleting a milestone while closing an issue in it", func(t *testing.T) {
		m := issueCreateMilestone(t, repo, "deleting")
		index, id := create(m)

		statuses := c.doBothWhileLocked("SELECT id FROM issue WHERE id = ? FOR UPDATE", id, closeIssue(index),
			lockedRequest{http.MethodDelete, issueAPIPath(repo, "milestones", m), map[string]any{}})
		assert.Equal(t, []int{http.StatusNoContent, http.StatusNoContent}, statuses)
		issue := issueFromDatabase(t, repo, index)
		assert.True(t, issue.IsClosed)
		assert.Zero(t, issue.MilestoneID)
	})

	t.Run("deleting a milestone while moving an issue to it", func(t *testing.T) {
		m := issueCreateMilestone(t, repo, "deleting-moved-to")
		other := issueCreateMilestone(t, repo, "deleting-moved-to-other")
		index, _ := create(0)

		// The move commits after the delete has listed the issues of the
		// milestone.
		statuses := c.doBothWhileLocked("SELECT id FROM milestone WHERE id = ? FOR UPDATE", m, moveTo(index, m),
			lockedRequest{http.MethodDelete, issueAPIPath(repo, "milestones", m), map[string]any{}})
		assert.Equal(t, []int{http.StatusNoContent, http.StatusNoContent}, statuses)
		left, err := database.Handle.Milestones().CountIssues(t.Context(), repo.ID, m, false, true)
		require.NoError(t, err)
		assert.Zero(t, left, "issues left with the deleted milestone")

		var view issueView
		c.doJSON(http.MethodGet, issueAPIPath(repo, "issues", index), nil, http.StatusOK, &view)
		assert.Nil(t, view.Milestone)
		for _, req := range []lockedRequest{closeIssue(index), moveTo(index, other)} {
			status, body := c.do(req.method, req.path, req.body)
			require.Equal(t, http.StatusNoContent, status, "%s %s: %s", req.method, req.path, body)
		}
		assert.Equal(t, []int{1, 1}, counts(other))
	})
}

// An edit whose client goes away after the content is saved still updates the
// files the content links.
func TestIssueEditAttachmentsClientGone(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	c := authenticatedClient(t, owner)
	upload := func() (uuid, link string) {
		t.Helper()
		status, body := issueUpload(t, c, issueAPIPath(repo, "issues", "attachments"), "screenshot.png", issuePNG(t))
		require.Equal(t, http.StatusCreated, status, "response: %s", body)
		var resp struct {
			UUID string `json:"uuid"`
			URL  string `json:"url"`
		}
		require.NoError(t, json.Unmarshal(body, &resp))
		return resp.UUID, "![screenshot](" + resp.URL + ")"
	}
	gone := func(uuid string) bool {
		_, err := database.Handle.Attachments().GetByUUID(t.Context(), uuid)
		_, statErr := os.Stat(database.AttachmentLocalPath(uuid))
		return database.IsErrAttachmentNotExist(err) && os.IsNotExist(statErr)
	}
	attachedTo := func(uuid string, issueID, commentID int64) bool {
		a, err := database.Handle.Attachments().GetByUUID(t.Context(), uuid)
		return err == nil && a.IssueID == issueID && a.CommentID == commentID
	}

	oldUUID, oldLink := upload()
	var created struct {
		Index int64 `json:"index"`
	}
	c.doJSON(http.MethodPost, issueAPIPath(repo, "issues"), map[string]any{"title": "Screenshot", "content": oldLink, "attachments": []string{oldUUID}}, http.StatusCreated, &created)
	issue := issueFromDatabase(t, repo, created.Index)
	require.True(t, attachedTo(oldUUID, issue.ID, 0))

	newUUID, newLink := upload()
	c.goAwayWhileLocked("SELECT id FROM issue WHERE id = ? FOR UPDATE", issue.ID,
		http.MethodPatch, issueAPIPath(repo, "issues", created.Index), map[string]any{"content": newLink, "attachments": []string{newUUID}})
	require.Eventually(t, func() bool {
		return gone(oldUUID) && attachedTo(newUUID, issue.ID, 0)
	}, 10*time.Second, 100*time.Millisecond, "files of the issue not updated")
	assert.Equal(t, newLink, issueFromDatabase(t, repo, created.Index).Content)

	oldUUID, oldLink = upload()
	var comment struct {
		ID int64 `json:"id"`
	}
	c.doJSON(http.MethodPost, issueAPIPath(repo, "issues", created.Index, "comments"), map[string]any{"content": oldLink, "attachments": []string{oldUUID}}, http.StatusCreated, &comment)
	require.True(t, attachedTo(oldUUID, issue.ID, comment.ID))

	newUUID, newLink = upload()
	c.goAwayWhileLocked("SELECT id FROM comment WHERE id = ? FOR UPDATE", comment.ID,
		http.MethodPatch, issueAPIPath(repo, "issues", created.Index, "comments", comment.ID), map[string]any{"content": newLink, "attachments": []string{newUUID}})
	require.Eventually(t, func() bool {
		return gone(oldUUID) && attachedTo(newUUID, issue.ID, comment.ID)
	}, 10*time.Second, 100*time.Millisecond, "files of the comment not updated")

	c.goAwayWhileLocked("SELECT id FROM attachment WHERE uuid = ? FOR UPDATE", newUUID,
		http.MethodDelete, issueAPIPath(repo, "issues", created.Index, "comments", comment.ID), nil)
	require.Eventually(t, func() bool {
		_, err := database.Handle.Comments().GetByID(t.Context(), comment.ID)
		return database.IsErrCommentNotExist(err) && gone(newUUID)
	}, 10*time.Second, 100*time.Millisecond, "comment or its files not deleted")
}

func TestIssueAttachments(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	public := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	private := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
	ownerClient := authenticatedClient(t, owner)
	otherClient := authenticatedClient(t, other)
	anonymous := newClient(t)
	content := issuePNG(t)

	upload := func(repo *Repository) (uuid, url string) {
		t.Helper()

		status, body := issueUpload(t, ownerClient, issueAPIPath(repo, "issues", "attachments"), "screenshot.png", content)
		require.Equal(t, http.StatusCreated, status, "response: %s", body)
		var resp struct {
			UUID string `json:"uuid"`
			URL  string `json:"url"`
		}
		require.NoError(t, json.Unmarshal(body, &resp))
		require.NotEmpty(t, resp.UUID)
		assert.Equal(t, serverURL+"/attachments/"+resp.UUID, resp.URL)
		return resp.UUID, resp.URL
	}
	download := func(c *client, uuid string) (int, []byte) {
		t.Helper()

		status, _, body := c.doRaw(http.MethodGet, "/attachments/"+uuid, "", nil)
		return status, body
	}

	t.Run("upload limits", func(t *testing.T) {
		status, body := issueUpload(t, ownerClient, issueAPIPath(public, "issues", "attachments"), "notes.txt", []byte("plain text"))
		assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)
		status, body = issueUpload(t, anonymous, issueAPIPath(public, "issues", "attachments"), "screenshot.png", content)
		assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
		status, body = issueUpload(t, otherClient, issueAPIPath(private, "issues", "attachments"), "screenshot.png", content)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})

	t.Run("public repository", func(t *testing.T) {
		uuid, url := upload(public)
		a, err := database.Handle.Attachments().GetByUUID(t.Context(), uuid)
		require.NoError(t, err)
		assert.Equal(t, owner.ID, a.UploaderID)
		assert.Equal(t, "screenshot.png", a.Name)

		// Only the uploader can download a file that is not attached yet.
		status, body := download(ownerClient, uuid)
		require.Equal(t, http.StatusOK, status)
		assert.True(t, bytes.Equal(content, body), "downloaded file differs from the upload")
		status, _ = download(otherClient, uuid)
		assert.Equal(t, http.StatusNotFound, status)

		var created struct {
			ID int64 `json:"id"`
		}
		index := issueCreate(t, ownerClient, public, "Has a screenshot", "")
		ownerClient.doJSON(http.MethodPost, issueAPIPath(public, "issues", index, "comments"), map[string]any{
			"content":     "![screenshot](" + url + ")",
			"attachments": []string{uuid},
		}, http.StatusCreated, &created)
		a, err = database.Handle.Attachments().GetByUUID(t.Context(), uuid)
		require.NoError(t, err)
		assert.Equal(t, created.ID, a.CommentID)

		status, body = download(anonymous, uuid)
		require.Equal(t, http.StatusOK, status)
		assert.True(t, bytes.Equal(content, body), "downloaded file differs from the upload")

		// A file that is attached cannot be attached again elsewhere.
		status, body = ownerClient.do(http.MethodPost, issueAPIPath(public, "issues", index, "comments"), map[string]any{
			"content":     "Again",
			"attachments": []string{uuid},
		})
		assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)
	})

	t.Run("private repository", func(t *testing.T) {
		uuid, _ := upload(private)
		var created struct {
			Index int64 `json:"index"`
		}
		ownerClient.doJSON(http.MethodPost, issueAPIPath(private, "issues"), map[string]any{
			"title":       "Private screenshot",
			"attachments": []string{uuid},
		}, http.StatusCreated, &created)
		a, err := database.Handle.Attachments().GetByUUID(t.Context(), uuid)
		require.NoError(t, err)
		assert.Equal(t, issueFromDatabase(t, private, created.Index).ID, a.IssueID)

		status, body := download(ownerClient, uuid)
		require.Equal(t, http.StatusOK, status)
		assert.True(t, bytes.Equal(content, body), "downloaded file differs from the upload")
		for name, c := range map[string]*client{"anonymous": anonymous, "other": otherClient} {
			status, _ = download(c, uuid)
			assert.Equal(t, http.StatusNotFound, status, "%s viewer", name)
		}
	})
}

func TestIssueSuggestionsAndMarkdown(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	private := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
	ownerClient := authenticatedClient(t, owner)
	otherClient := authenticatedClient(t, other)

	t.Run("mentions", func(t *testing.T) {
		var resp struct {
			Items []struct {
				Name string `json:"name"`
			} `json:"items"`
		}
		otherClient.doJSON(http.MethodGet, issueAPIPath(private, "suggestions", "mentions"), nil, http.StatusNotFound, &struct{}{})
		ownerClient.doJSON(http.MethodGet, issueAPIPath(private, "suggestions", "mentions"), nil, http.StatusOK, &resp)
		var names []string
		for _, item := range resp.Items {
			names = append(names, item.Name)
		}
		assert.Contains(t, names, owner.Name)
		assert.NotContains(t, names, other.Name)
	})

	t.Run("issues", func(t *testing.T) {
		keyword := UniqueName("needle")
		match := issueCreate(t, ownerClient, repo, "Find the "+keyword, "")
		issueCreate(t, ownerClient, repo, "Unrelated", "")

		var resp struct {
			Items []struct {
				Index  int64  `json:"index"`
				Title  string `json:"title"`
				IsPull bool   `json:"isPull"`
			} `json:"items"`
		}
		otherClient.doJSON(http.MethodGet, issueAPIPath(repo, "suggestions", "issues")+"?q="+keyword, nil, http.StatusOK, &resp)
		require.Len(t, resp.Items, 1)
		assert.Equal(t, match, resp.Items[0].Index)
		assert.Equal(t, "Find the "+keyword, resp.Items[0].Title)
		assert.False(t, resp.Items[0].IsPull)

		status, body := newClient(t).do(http.MethodGet, issueAPIPath(repo, "suggestions", "issues"), nil)
		assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
		status, body = otherClient.do(http.MethodGet, issueAPIPath(private, "suggestions", "issues"), nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})

	t.Run("markdown", func(t *testing.T) {
		var resp struct {
			HTML string `json:"html"`
		}
		otherClient.doJSON(http.MethodPost, issueAPIPath(repo, "markdown"), map[string]any{"text": "Some **bold** and [a link](docs/guide.md)"}, http.StatusOK, &resp)
		assert.Contains(t, resp.HTML, "<strong>bold</strong>")
		assert.Contains(t, resp.HTML, `href="/`+repo.Owner+"/"+repo.Name+`/docs/guide.md"`)

		ownerClient.doJSON(http.MethodPost, issueAPIPath(repo, "markdown"), map[string]any{"text": "[guide](guide.md)", "wiki": true}, http.StatusOK, &resp)
		assert.Contains(t, resp.HTML, `href="/`+repo.Owner+"/"+repo.Name+`/wiki/guide.md"`)

		status, body := newClient(t).do(http.MethodPost, issueAPIPath(repo, "markdown"), map[string]any{"text": "x"})
		assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
		status, body = otherClient.do(http.MethodPost, issueAPIPath(private, "markdown"), map[string]any{"text": "x"})
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})
}

func TestIssueMarkdownCommitLinks(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	other := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	ownerClient := authenticatedClient(t, owner)

	dir := database.RepoPath(repo.Owner, repo.Name)
	commitID := repoSettingsGit(t, dir, "rev-parse", "HEAD")
	short, abbrev9, abbrev10 := issueCommitAbbrev(commitID, 7), issueCommitAbbrev(commitID, 9), issueCommitAbbrev(commitID, 10)
	otherCommitID := repoSettingsAddCommit(t, other, repoSettingsDefaultBranch(t, other), "Only in the other repository")
	// Git resolves a hex string that is also the name of a branch or tag to the
	// commit it points to, which is not the commit the hex string names.
	const hexBranch = "cafe1234"
	repoSettingsGit(t, dir, "update-ref", "refs/heads/"+hexBranch, commitID)

	link := func(id, text string) string {
		return `<a href="/` + repo.Owner + "/" + repo.Name + "/commit/" + id + `" rel="nofollow"><code>` + text + "</code></a>"
	}

	content := "Fixed in " + short + ", not in 123e4567-e89b-12d3-a456-426614174000 or deadbeef. " +
		"Branch " + hexBranch + ". Other " + otherCommitID + "."
	index := issueCreate(t, ownerClient, repo, "Commit links", content)
	// Comments are rendered with the commits of the whole issue, which are
	// resolved together.
	var created struct {
		ID int64 `json:"id"`
	}
	ownerClient.doJSON(http.MethodPost, issueAPIPath(repo, "issues", index, "comments"), map[string]any{"content": "Also " + abbrev9 + ", not `" + commitID[:8] + "`."}, http.StatusCreated, &created)
	v := issueGet(t, ownerClient, repo, index)
	require.Len(t, v.Comments, 1)
	assert.Contains(t, v.Comments[0].ContentHTML, "Also "+link(commitID, abbrev9[:min(len(abbrev9), 10)])+", not <code>"+commitID[:8]+"</code>.")
	got := v.ContentHTML
	assert.Contains(t, got, "Fixed in "+link(commitID, short)+",", "a short hash links to the full commit ID")
	assert.Contains(t, got, "not in 123e4567-e89b-12d3-a456-426614174000 or deadbeef.")
	assert.Contains(t, got, "Branch "+hexBranch+".")
	assert.Contains(t, got, "Other "+otherCommitID+".", "a commit of another repository stays text")
	assert.Equal(t, 1, strings.Count(got, "<a "), "content HTML: %s", got)

	var resp struct {
		HTML string `json:"html"`
	}
	ownerClient.doJSON(http.MethodPost, issueAPIPath(repo, "markdown"), map[string]any{"text": "修复了" + abbrev10 + "的问题", "wiki": true}, http.StatusOK, &resp)
	assert.Contains(t, resp.HTML, "修复了"+link(commitID, abbrev10[:10])+"的问题")

	t.Run("guests who cannot read the code", func(t *testing.T) {
		private := seedRepository(t, owner.ID, CreateRepositoryOptions{Private: true, AutoInit: true})
		repoVisibilityPutPublicAccess(t, ownerClient, private, false, true)
		privateID := repoSettingsGit(t, database.RepoPath(private.Owner, private.Name), "rev-parse", "HEAD")
		privateShort := issueCommitAbbrev(privateID, 7)
		privateLink := `<a href="/` + private.Owner + "/" + private.Name + "/commit/" + privateID + `" rel="nofollow"><code>` + privateShort[:min(len(privateShort), 10)] + "</code></a>"
		guestClient := authenticatedClient(t, seedUser(t, CreateUserOptions{}))

		// Links to the full ID would tell guests which commits the code has.
		index := issueCreate(t, ownerClient, private, "Commit links", "Fixed in "+privateShort+".")
		assert.Contains(t, issueGet(t, ownerClient, private, index).ContentHTML, "Fixed in "+privateLink+".")
		assert.Contains(t, issueGet(t, guestClient, private, index).ContentHTML, "Fixed in "+privateShort+".")

		var resp struct {
			HTML string `json:"html"`
		}
		guestClient.doJSON(http.MethodPost, issueAPIPath(private, "markdown"), map[string]any{"text": "Probe " + privateShort}, http.StatusOK, &resp)
		assert.Equal(t, "<p>Probe "+privateShort+"</p>\n", resp.HTML)
	})
}

// issueCommitAbbrev returns the shortest abbreviation of the commit ID that is
// at least n characters long and not a number, which never links.
func issueCommitAbbrev(id string, n int) string {
	for strings.Trim(id[:n], "0123456789") == "" {
		n++
	}
	return id[:n]
}
