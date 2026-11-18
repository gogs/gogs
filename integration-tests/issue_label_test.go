package integrationtests

import (
	"context"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/database"
)

// issueCreateLabel seeds a label in the repository and returns its ID.
func issueCreateLabel(t *testing.T, repo *Repository, name string) int64 {
	t.Helper()

	label := &database.Label{RepoID: repo.ID, Name: name, Color: "#123456"}
	require.NoError(t, database.Handle.Labels().Create(t.Context(), label))
	return label.ID
}

// issueCreateMilestone seeds a milestone without a due date in the repository
// and returns its ID.
func issueCreateMilestone(t *testing.T, repo *Repository, name string) int64 {
	t.Helper()

	m := &database.Milestone{RepoID: repo.ID, Name: name, Deadline: time.Date(9999, 12, 31, 0, 0, 0, 0, time.Local)}
	require.NoError(t, database.Handle.Milestones().Create(t.Context(), m))
	return m.ID
}

func issueLabelsOf(t *testing.T, repo *Repository) map[string]*database.Label {
	t.Helper()

	labels, err := database.Handle.Labels().ListByRepoID(t.Context(), repo.ID)
	require.NoError(t, err)
	byName := make(map[string]*database.Label, len(labels))
	for _, l := range labels {
		byName[l.Name] = l
	}
	return byName
}

func TestIssueRepositoryLabels(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	ownerClient := authenticatedClient(t, owner)
	otherClient := authenticatedClient(t, other)

	type labelList struct {
		Items []struct {
			ID             int64  `json:"id"`
			Name           string `json:"name"`
			Color          string `json:"color"`
			OpenIssueCount int    `json:"openIssueCount"`
		} `json:"items"`
		Templates      []string `json:"templates"`
		ViewerCanWrite bool     `json:"viewerCanWrite"`
	}

	status, body := ownerClient.do(http.MethodPost, issueAPIPath(repo, "labels"), map[string]any{"name": "bug", "color": "#ee0701"})
	require.Equal(t, http.StatusCreated, status, "response: %s", body)
	bug := issueLabelsOf(t, repo)["bug"]
	require.NotNil(t, bug)
	assert.Equal(t, "#ee0701", bug.Color)

	t.Run("create validation and access", func(t *testing.T) {
		status, body := ownerClient.do(http.MethodPost, issueAPIPath(repo, "labels"), map[string]any{"name": "bad", "color": "red"})
		assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)
		assert.Contains(t, string(body), `"color"`)
		status, body = ownerClient.do(http.MethodPost, issueAPIPath(repo, "labels"), map[string]any{"name": "", "color": "#000000"})
		assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)
		status, body = otherClient.do(http.MethodPost, issueAPIPath(repo, "labels"), map[string]any{"name": "intruder", "color": "#000000"})
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
		status, body = newClient(t).do(http.MethodPost, issueAPIPath(repo, "labels"), map[string]any{"name": "intruder", "color": "#000000"})
		assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
		assert.Len(t, issueLabelsOf(t, repo), 1)
	})

	t.Run("list", func(t *testing.T) {
		index := issueCreate(t, ownerClient, repo, "Labeled", "")
		status, body := ownerClient.do(http.MethodPut, issueAPIPath(repo, "issues", index, "labels"), map[string]any{"ids": []int64{bug.ID}})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)

		var list labelList
		newClient(t).doJSON(http.MethodGet, issueAPIPath(repo, "labels"), nil, http.StatusOK, &list)
		require.Len(t, list.Items, 1)
		assert.Equal(t, bug.ID, list.Items[0].ID)
		assert.Equal(t, "bug", list.Items[0].Name)
		assert.Equal(t, 1, list.Items[0].OpenIssueCount)
		assert.False(t, list.ViewerCanWrite)
		assert.Empty(t, list.Templates)

		ownerClient.doJSON(http.MethodGet, issueAPIPath(repo, "labels"), nil, http.StatusOK, &list)
		assert.True(t, list.ViewerCanWrite)
		assert.Contains(t, list.Templates, "Default")
	})

	t.Run("update", func(t *testing.T) {
		path := issueAPIPath(repo, "labels", bug.ID)
		status, body := otherClient.do(http.MethodPatch, path, map[string]any{"name": "hijacked", "color": "#000000"})
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
		status, body = ownerClient.do(http.MethodPatch, path, map[string]any{"name": "defect", "color": "#00FF00"})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		label, err := database.Handle.Labels().GetByID(t.Context(), repo.ID, bug.ID)
		require.NoError(t, err)
		assert.Equal(t, "defect", label.Name)
		assert.Equal(t, "#00FF00", label.Color)

		// A label of another repository is not found through this one.
		otherRepo := seedRepository(t, owner.ID, CreateRepositoryOptions{})
		status, body = ownerClient.do(http.MethodPatch, issueAPIPath(otherRepo, "labels", bug.ID), map[string]any{"name": "moved", "color": "#000000"})
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})

	t.Run("delete", func(t *testing.T) {
		id := issueCreateLabel(t, repo, "temporary")
		path := issueAPIPath(repo, "labels", id)
		status, body := otherClient.do(http.MethodDelete, path, map[string]any{})
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
		status, body = ownerClient.do(http.MethodDelete, path, map[string]any{})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		_, err := database.Handle.Labels().GetByID(t.Context(), repo.ID, id)
		assert.True(t, database.IsErrLabelNotExist(err), "label not deleted: %v", err)
	})
}

func TestIssueInitializeLabels(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{})
	ownerClient := authenticatedClient(t, owner)
	otherClient := authenticatedClient(t, other)
	path := issueAPIPath(repo, "labels", "initialize")

	status, body := otherClient.do(http.MethodPost, path, map[string]any{"template": "Default"})
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	status, body = ownerClient.do(http.MethodPost, path, map[string]any{"template": "../../etc/passwd"})
	assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)
	assert.Empty(t, issueLabelsOf(t, repo))

	status, body = ownerClient.do(http.MethodPost, path, map[string]any{"template": "Default"})
	require.Equal(t, http.StatusCreated, status, "response: %s", body)
	// The labels of conf/label/Default.
	want := map[string]string{
		"bug":         "#ee0701",
		"duplicate":   "#cccccc",
		"enhancement": "#84b6eb",
		"help wanted": "#128a0c",
		"invalid":     "#e6e6e6",
		"question":    "#cc317c",
		"wontfix":     "#ffffff",
	}
	got := make(map[string]string)
	for name, l := range issueLabelsOf(t, repo) {
		got[name] = l.Color
	}
	assert.Equal(t, want, got)
}

func TestIssueMilestones(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	ownerClient := authenticatedClient(t, owner)
	otherClient := authenticatedClient(t, other)

	type milestoneList struct {
		Items []struct {
			ID              int64  `json:"id"`
			Name            string `json:"name"`
			DescriptionHTML string `json:"descriptionHTML"`
			Deadline        string `json:"deadline"`
			IsClosed        bool   `json:"isClosed"`
			OpenIssueCount  int    `json:"openIssueCount"`
		} `json:"items"`
		OpenCount      int64 `json:"openCount"`
		ClosedCount    int64 `json:"closedCount"`
		ViewerCanWrite bool  `json:"viewerCanWrite"`
	}
	list := func(c *client, state string) milestoneList {
		t.Helper()

		var l milestoneList
		c.doJSON(http.MethodGet, issueAPIPath(repo, "milestones")+"?state="+state, nil, http.StatusOK, &l)
		return l
	}

	status, body := ownerClient.do(http.MethodPost, issueAPIPath(repo, "milestones"), map[string]any{
		"title":       "v1.0",
		"description": "The **first** release",
		"deadline":    "2099-06-30",
	})
	require.Equal(t, http.StatusCreated, status, "response: %s", body)

	open := list(newClient(t), "open")
	require.Len(t, open.Items, 1)
	m := open.Items[0]
	assert.Equal(t, "v1.0", m.Name)
	assert.Contains(t, m.DescriptionHTML, "<strong>first</strong>")
	assert.Equal(t, "2099-06-30", m.Deadline)
	assert.False(t, m.IsClosed)
	assert.Equal(t, int64(1), open.OpenCount)
	assert.False(t, open.ViewerCanWrite)
	assert.True(t, list(ownerClient, "open").ViewerCanWrite)
	path := issueAPIPath(repo, "milestones", m.ID)

	t.Run("create validation and access", func(t *testing.T) {
		status, body := ownerClient.do(http.MethodPost, issueAPIPath(repo, "milestones"), map[string]any{"title": "Bad date", "deadline": "30/06/2099"})
		assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)
		assert.Contains(t, string(body), `"deadline"`)
		status, body = otherClient.do(http.MethodPost, issueAPIPath(repo, "milestones"), map[string]any{"title": "Intruder"})
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
		assert.Len(t, list(ownerClient, "open").Items, 1)
	})

	t.Run("get", func(t *testing.T) {
		var got struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			Deadline    string `json:"deadline"`
		}
		ownerClient.doJSON(http.MethodGet, path, nil, http.StatusOK, &got)
		assert.Equal(t, "v1.0", got.Title)
		assert.Equal(t, "The **first** release", got.Description)
		assert.Equal(t, "2099-06-30", got.Deadline)

		status, body := otherClient.do(http.MethodGet, path, nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
		status, body = ownerClient.do(http.MethodGet, issueAPIPath(repo, "milestones", int64(0)), nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})

	t.Run("update", func(t *testing.T) {
		status, body := otherClient.do(http.MethodPatch, path, map[string]any{"title": "Hijacked"})
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
		status, body = ownerClient.do(http.MethodPatch, path, map[string]any{"title": "v1.1", "description": "Renamed"})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		got, err := database.Handle.Milestones().GetByID(t.Context(), repo.ID, m.ID)
		require.NoError(t, err)
		assert.Equal(t, "v1.1", got.Name)
		assert.Equal(t, "Renamed", got.Content)
		assert.Empty(t, got.DeadlineString, "a milestone without a deadline has none")
	})

	t.Run("close and reopen", func(t *testing.T) {
		status, body := otherClient.do(http.MethodPut, path+"/closed", map[string]any{})
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

		status, body = ownerClient.do(http.MethodPut, path+"/closed", map[string]any{})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		got, err := database.Handle.Milestones().GetByID(t.Context(), repo.ID, m.ID)
		require.NoError(t, err)
		assert.True(t, got.IsClosed)
		assert.Empty(t, list(ownerClient, "open").Items)
		closed := list(ownerClient, "closed")
		require.Len(t, closed.Items, 1)
		assert.Equal(t, m.ID, closed.Items[0].ID)
		assert.True(t, closed.Items[0].IsClosed)

		status, body = ownerClient.do(http.MethodDelete, path+"/closed", map[string]any{})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		got, err = database.Handle.Milestones().GetByID(t.Context(), repo.ID, m.ID)
		require.NoError(t, err)
		assert.False(t, got.IsClosed)
		assert.Len(t, list(ownerClient, "open").Items, 1)
	})

	t.Run("delete", func(t *testing.T) {
		status, body := otherClient.do(http.MethodDelete, path, map[string]any{})
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

		index := issueCreate(t, ownerClient, repo, "In the milestone", "")
		status, body = ownerClient.do(http.MethodPut, issueAPIPath(repo, "issues", index, "milestone"), map[string]any{"id": m.ID})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)

		status, body = ownerClient.do(http.MethodDelete, path, map[string]any{})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		_, err := database.Handle.Milestones().GetByID(t.Context(), repo.ID, m.ID)
		assert.True(t, database.IsErrMilestoneNotExist(err), "milestone not deleted: %v", err)
		assert.Zero(t, issueFromDatabase(t, repo, index).MilestoneID, "the issue still has the deleted milestone")
		names := []string{}
		for _, item := range list(ownerClient, "open").Items {
			names = append(names, item.Name)
		}
		assert.False(t, slices.Contains(names, "v1.1"))
	})
}

// The numbers of issues of labels and milestones keep the issues another
// change counts while a request waits to count its own.
func TestIssueCountsUnderConcurrentChanges(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{})
	api := apiV1Client(t, owner)
	labelID := issueCreateLabel(t, repo, "bug")
	milestoneID := issueCreateMilestone(t, repo, "v1")
	labeled := apiV1CreateIssue(t, api, repo, map[string]any{"title": "Labeled"})
	inMilestone := apiV1CreateIssue(t, api, repo, map[string]any{"title": "In milestone"})

	countsOf := func(table string, id int64) []int {
		t.Helper()
		var counts struct {
			NumIssues       int
			NumClosedIssues int
			Completeness    int
		}
		columns := "num_issues, num_closed_issues"
		if table == "milestone" {
			columns += ", completeness"
		}
		require.NoError(t, gormDB.Raw("SELECT "+columns+" FROM "+table+" WHERE id = ?", id).Scan(&counts).Error)
		if table == "milestone" {
			return []int{counts.NumIssues, counts.NumClosedIssues, counts.Completeness}
		}
		return []int{counts.NumIssues, counts.NumClosedIssues}
	}
	// whileCounting sends the request while the row of the label or milestone
	// is locked by a transaction that counts a closed issue of another change.
	whileCounting := func(table string, id int64, method, path string, body any) {
		t.Helper()
		status, resp := api.whileLocked("SELECT id FROM "+table+" WHERE id = ? FOR UPDATE", id, method, apiV1RepoPath(repo, path), body,
			func(tx *gorm.DB, _ context.CancelFunc) {
				require.NoError(t, tx.Exec("UPDATE "+table+" SET num_issues = num_issues + 1, num_closed_issues = num_closed_issues + 1 WHERE id = ?", id).Error)
			},
		)
		require.True(t, status >= 200 && status < 300, "status %d, response: %s", status, resp)
	}

	whileCounting("label", labelID, http.MethodPost, "/issues/"+apiV1Itoa(labeled)+"/labels", map[string]any{"labels": []int64{labelID}})
	assert.Equal(t, []int{2, 1}, countsOf("label", labelID), "label added to an issue")
	whileCounting("label", labelID, http.MethodPatch, "/issues/"+apiV1Itoa(labeled), map[string]any{"state": "closed"})
	assert.Equal(t, []int{3, 3}, countsOf("label", labelID), "issue with the label closed")
	whileCounting("label", labelID, http.MethodPatch, "/labels/"+apiV1Itoa(labelID), map[string]any{"name": "defect"})
	assert.Equal(t, []int{4, 4}, countsOf("label", labelID), "label edited")
	whileCounting("label", labelID, http.MethodDelete, "/issues/"+apiV1Itoa(labeled)+"/labels/"+apiV1Itoa(labelID), nil)
	assert.Equal(t, []int{4, 4}, countsOf("label", labelID), "label removed from a closed issue")

	whileCounting("milestone", milestoneID, http.MethodPatch, "/issues/"+apiV1Itoa(inMilestone), map[string]any{"milestone": milestoneID})
	assert.Equal(t, []int{2, 1, 50}, countsOf("milestone", milestoneID), "issue added to the milestone")
	whileCounting("milestone", milestoneID, http.MethodPatch, "/issues/"+apiV1Itoa(inMilestone), map[string]any{"state": "closed"})
	assert.Equal(t, []int{3, 3, 100}, countsOf("milestone", milestoneID), "issue in the milestone closed")
	whileCounting("milestone", milestoneID, http.MethodPatch, "/milestones/"+apiV1Itoa(milestoneID), map[string]any{"title": "v1.0", "state": "closed"})
	assert.Equal(t, []int{4, 4, 100}, countsOf("milestone", milestoneID), "milestone edited and closed")
	whileCounting("milestone", milestoneID, http.MethodPatch, "/issues/"+apiV1Itoa(inMilestone), map[string]any{"milestone": 0})
	assert.Equal(t, []int{4, 4, 100}, countsOf("milestone", milestoneID), "closed issue removed from the milestone")
}
