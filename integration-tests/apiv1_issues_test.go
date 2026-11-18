package integrationtests

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/database"
)

type apiV1Label struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type apiV1Milestone struct {
	ID           int64      `json:"id"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	State        string     `json:"state"`
	OpenIssues   int        `json:"open_issues"`
	ClosedIssues int        `json:"closed_issues"`
	Closed       *time.Time `json:"closed_at"`
	Deadline     *time.Time `json:"due_on"`
}

type apiV1Issue struct {
	Number    int64           `json:"number"`
	Title     string          `json:"title"`
	Body      string          `json:"body"`
	State     string          `json:"state"`
	User      apiV1User       `json:"user"`
	Assignee  *apiV1User      `json:"assignee"`
	Milestone *apiV1Milestone `json:"milestone"`
	Labels    []apiV1Label    `json:"labels"`
	Comments  int             `json:"comments"`
}

func apiV1IssueTitles(issues []apiV1Issue) []string {
	titles := []string{}
	for _, issue := range issues {
		titles = append(titles, issue.Title)
	}
	return titles
}

func TestAPIV1Issues(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	api := apiV1Client(t, owner)

	var milestone apiV1Milestone
	api.doJSON(http.MethodPost, apiV1RepoPath(repo, "/milestones"), map[string]any{"title": "v1"}, http.StatusCreated, &milestone)
	open := apiV1CreateIssue(t, api, repo, map[string]any{"title": "Open issue", "body": "It **crashes**.", "assignee": owner.Name})
	closed := apiV1CreateIssue(t, api, repo, map[string]any{"title": "Closed issue", "closed": true})

	var issues []apiV1Issue
	api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/issues"), nil, http.StatusOK, &issues)
	assert.Equal(t, []string{"Open issue"}, apiV1IssueTitles(issues))
	issues = nil
	api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/issues?state=closed"), nil, http.StatusOK, &issues)
	assert.Equal(t, []string{"Closed issue"}, apiV1IssueTitles(issues))

	var issue apiV1Issue
	api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/issues/"+apiV1Itoa(open)), nil, http.StatusOK, &issue)
	assert.Equal(t, open, issue.Number)
	assert.Equal(t, "Open issue", issue.Title)
	assert.Equal(t, "It **crashes**.", issue.Body)
	assert.Equal(t, "open", issue.State)
	assert.Equal(t, owner.Name, issue.User.UserName)
	require.NotNil(t, issue.Assignee)
	assert.Equal(t, owner.Name, issue.Assignee.UserName)

	// The web app shows the issue as created through API v1.
	v := issueGet(t, newClient(t), repo, open)
	assert.Equal(t, "Open issue", v.Title)
	assert.Contains(t, v.ContentHTML, "<strong>crashes</strong>")
	assert.Equal(t, owner.Name, v.Poster.Name)

	other := seedUser(t, CreateUserOptions{})
	otherAPI := apiV1Client(t, other)
	status, body := otherAPI.do(http.MethodPatch, apiV1RepoPath(repo, "/issues/"+apiV1Itoa(open)), map[string]any{"title": "Hijacked"})
	assert.Equal(t, http.StatusForbidden, status, "edited by another user, response: %s", body)

	issue = apiV1Issue{}
	api.doJSON(http.MethodPatch, apiV1RepoPath(repo, "/issues/"+apiV1Itoa(open)), map[string]any{
		"title":     "Edited issue",
		"body":      "Edited body",
		"assignee":  "",
		"milestone": milestone.ID,
		"state":     "closed",
	}, http.StatusCreated, &issue)
	assert.Equal(t, "Edited issue", issue.Title)

	issue = apiV1Issue{}
	api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/issues/"+apiV1Itoa(open)), nil, http.StatusOK, &issue)
	assert.Equal(t, "Edited issue", issue.Title)
	assert.Equal(t, "Edited body", issue.Body)
	assert.Equal(t, "closed", issue.State)
	assert.Nil(t, issue.Assignee)
	require.NotNil(t, issue.Milestone)
	assert.Equal(t, milestone.ID, issue.Milestone.ID)

	status, body = api.do(http.MethodGet, apiV1RepoPath(repo, "/issues/"+apiV1Itoa(closed+100)), nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

	private := seedRepository(t, owner.ID, CreateRepositoryOptions{Private: true})
	status, body = otherAPI.do(http.MethodGet, apiV1RepoPath(private, "/issues"), nil)
	assert.Equal(t, http.StatusNotFound, status, "private repository of another user, response: %s", body)
}

func TestAPIV1AssignedIssues(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	assignee := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{})
	api := apiV1Client(t, owner)
	apiV1CreateIssue(t, api, repo, map[string]any{"title": "Assigned", "assignee": assignee.Name})
	apiV1CreateIssue(t, api, repo, map[string]any{"title": "Assigned and closed", "assignee": assignee.Name, "closed": true})
	apiV1CreateIssue(t, api, repo, map[string]any{"title": "Not assigned"})

	assigneeAPI := apiV1Client(t, assignee)
	for _, path := range []string{"/api/v1/issues", "/api/v1/user/issues"} {
		var issues []apiV1Issue
		assigneeAPI.doJSON(http.MethodGet, path, nil, http.StatusOK, &issues)
		assert.Equal(t, []string{"Assigned"}, apiV1IssueTitles(issues), path)

		issues = nil
		assigneeAPI.doJSON(http.MethodGet, path+"?state=closed", nil, http.StatusOK, &issues)
		assert.Equal(t, []string{"Assigned and closed"}, apiV1IssueTitles(issues), path)

		status, body := newClient(t).do(http.MethodGet, path, nil)
		assert.Equal(t, http.StatusUnauthorized, status, "%s, response: %s", path, body)
	}
}

type apiV1Comment struct {
	ID   int64     `json:"id"`
	Body string    `json:"body"`
	User apiV1User `json:"user"`
}

func TestAPIV1IssueComments(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	commenter := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{})
	ownerAPI := apiV1Client(t, owner)
	commenterAPI := apiV1Client(t, commenter)
	index := apiV1CreateIssue(t, ownerAPI, repo, map[string]any{"title": "Discuss"})
	issuePath := apiV1RepoPath(repo, "/issues/"+apiV1Itoa(index))

	var created apiV1Comment
	commenterAPI.doJSON(http.MethodPost, issuePath+"/comments", map[string]any{"body": "First"}, http.StatusCreated, &created)
	assert.Equal(t, "First", created.Body)
	assert.Equal(t, commenter.Name, created.User.UserName)

	listComments := func(path string) []apiV1Comment {
		t.Helper()

		var comments []apiV1Comment
		ownerAPI.doJSON(http.MethodGet, path, nil, http.StatusOK, &comments)
		return comments
	}
	assert.Equal(t, []apiV1Comment{created}, listComments(issuePath+"/comments"))
	assert.Equal(t, []apiV1Comment{created}, listComments(apiV1RepoPath(repo, "/issues/comments")))
	assert.Empty(t, listComments(apiV1RepoPath(repo, "/issues/comments?since="+time.Now().Add(time.Hour).Format(time.RFC3339))))

	var edited apiV1Comment
	commenterAPI.doJSON(http.MethodPatch, issuePath+"/comments/"+apiV1Itoa(created.ID), map[string]any{"body": "Edited"}, http.StatusOK, &edited)
	assert.Equal(t, "Edited", edited.Body)
	assert.Equal(t, "Edited", listComments(issuePath + "/comments")[0].Body)

	edited = apiV1Comment{}
	commenterAPI.doJSON(http.MethodPatch, apiV1RepoPath(repo, "/issues/comments/"+apiV1Itoa(created.ID)), map[string]any{"body": "Edited again"}, http.StatusOK, &edited)
	assert.Equal(t, "Edited again", edited.Body)
	assert.Equal(t, "Edited again", listComments(issuePath + "/comments")[0].Body)

	stranger := apiV1Client(t, seedUser(t, CreateUserOptions{}))
	status, body := stranger.do(http.MethodPatch, apiV1RepoPath(repo, "/issues/comments/"+apiV1Itoa(created.ID)), map[string]any{"body": "Hijacked"})
	assert.Equal(t, http.StatusForbidden, status, "edited by another user, response: %s", body)
	status, body = stranger.do(http.MethodDelete, issuePath+"/comments/"+apiV1Itoa(created.ID), nil)
	assert.Equal(t, http.StatusForbidden, status, "deleted by another user, response: %s", body)

	status, body = commenterAPI.do(http.MethodDelete, issuePath+"/comments/"+apiV1Itoa(created.ID), nil)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.Empty(t, listComments(issuePath+"/comments"))
	_, err := database.GetCommentByID(created.ID)
	assert.True(t, database.IsErrCommentNotExist(err), "comment not deleted: %v", err)
}

func TestAPIV1Labels(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{})
	api := apiV1Client(t, owner)

	var resp apiV1Label
	api.doJSON(http.MethodPost, apiV1RepoPath(repo, "/labels"), map[string]any{"name": "bug", "color": "#ee0701"}, http.StatusCreated, &resp)
	assert.Equal(t, "bug", resp.Name)
	assert.Equal(t, "ee0701", resp.Color)

	var labels []apiV1Label
	api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/labels"), nil, http.StatusOK, &labels)
	require.Len(t, labels, 1)
	created := labels[0]
	assert.Equal(t, apiV1Label{ID: created.ID, Name: "bug", Color: "ee0701"}, created)
	t.Run("created ID", func(t *testing.T) {
		assert.Equal(t, created.ID, resp.ID)
	})

	for _, id := range []string{apiV1Itoa(created.ID), "bug"} {
		var got apiV1Label
		api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/labels/"+id), nil, http.StatusOK, &got)
		assert.Equal(t, created, got, "label %q", id)
	}

	var edited apiV1Label
	api.doJSON(http.MethodPatch, apiV1RepoPath(repo, "/labels/"+apiV1Itoa(created.ID)), map[string]any{"name": "defect", "color": "#000000"}, http.StatusOK, &edited)
	want := apiV1Label{ID: created.ID, Name: "defect", Color: "000000"}
	assert.Equal(t, want, edited)
	var got apiV1Label
	api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/labels/"+apiV1Itoa(created.ID)), nil, http.StatusOK, &got)
	assert.Equal(t, want, got)

	other := apiV1Client(t, seedUser(t, CreateUserOptions{}))
	status, body := other.do(http.MethodPost, apiV1RepoPath(repo, "/labels"), map[string]any{"name": "spam", "color": "#ffffff"})
	assert.Equal(t, http.StatusForbidden, status, "created by another user, response: %s", body)
	status, body = api.do(http.MethodPost, apiV1RepoPath(repo, "/labels"), map[string]any{"name": "bad color", "color": "red"})
	assert.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", body)

	status, body = api.do(http.MethodDelete, apiV1RepoPath(repo, "/labels/"+apiV1Itoa(created.ID)), nil)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	status, body = api.do(http.MethodGet, apiV1RepoPath(repo, "/labels/"+apiV1Itoa(created.ID)), nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
}

func TestAPIV1IssueLabels(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{})
	api := apiV1Client(t, owner)
	index := apiV1CreateIssue(t, api, repo, map[string]any{"title": "Labeled"})
	path := apiV1RepoPath(repo, "/issues/"+apiV1Itoa(index)+"/labels")

	newLabel := func(name string) apiV1Label {
		t.Helper()

		var l apiV1Label
		api.doJSON(http.MethodPost, apiV1RepoPath(repo, "/labels"), map[string]any{"name": name, "color": "#123456"}, http.StatusCreated, &l)
		// The response of the creation lacks the ID.
		api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/labels/"+name), nil, http.StatusOK, &l)
		require.NotZero(t, l.ID)
		return l
	}
	a, b, c := newLabel("a"), newLabel("b"), newLabel("c")
	listLabels := func() []apiV1Label {
		t.Helper()

		var labels []apiV1Label
		api.doJSON(http.MethodGet, path, nil, http.StatusOK, &labels)
		return labels
	}

	var labels []apiV1Label
	api.doJSON(http.MethodPost, path, map[string]any{"labels": []int64{a.ID}}, http.StatusOK, &labels)
	assert.Equal(t, []apiV1Label{a}, labels)
	assert.Equal(t, []apiV1Label{a}, listLabels())

	labels = nil
	api.doJSON(http.MethodPut, path, map[string]any{"labels": []int64{b.ID, c.ID}}, http.StatusOK, &labels)
	assert.ElementsMatch(t, []apiV1Label{b, c}, labels)
	assert.ElementsMatch(t, []apiV1Label{b, c}, listLabels())

	status, body := apiV1Client(t, seedUser(t, CreateUserOptions{})).do(http.MethodPost, path, map[string]any{"labels": []int64{a.ID}})
	assert.Equal(t, http.StatusForbidden, status, "added by another user, response: %s", body)

	status, body = api.do(http.MethodDelete, path+"/"+apiV1Itoa(b.ID), nil)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.Equal(t, []apiV1Label{c}, listLabels())

	status, body = api.do(http.MethodDelete, path, nil)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.Empty(t, listLabels())
}

func TestAPIV1Milestones(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{})
	api := apiV1Client(t, owner)
	countsOf := func() [2]int {
		t.Helper()
		r := repoSettingsGetRepo(t, repo.ID)
		return [2]int{r.NumMilestones, r.NumClosedMilestones}
	}

	deadline := time.Date(2030, 1, 2, 0, 0, 0, 0, time.UTC)
	var created apiV1Milestone
	api.doJSON(http.MethodPost, apiV1RepoPath(repo, "/milestones"), map[string]any{
		"title":       "v1",
		"description": "First",
		"due_on":      deadline,
	}, http.StatusCreated, &created)
	assert.Equal(t, "v1", created.Title)
	assert.Equal(t, "First", created.Description)
	assert.Equal(t, "open", created.State)
	require.NotNil(t, created.Deadline)
	assert.True(t, deadline.Equal(*created.Deadline), "deadline %v", created.Deadline)

	var milestones []apiV1Milestone
	api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/milestones"), nil, http.StatusOK, &milestones)
	require.Len(t, milestones, 1)
	assert.Equal(t, created.ID, milestones[0].ID)
	assert.Equal(t, "v1", milestones[0].Title)

	assert.Equal(t, [2]int{1, 0}, countsOf())

	otherRepo := seedRepository(t, owner.ID, CreateRepositoryOptions{})
	var ofOtherRepo apiV1Milestone
	api.doJSON(http.MethodPost, apiV1RepoPath(otherRepo, "/milestones"), map[string]any{"title": "Elsewhere"}, http.StatusCreated, &ofOtherRepo)
	for _, id := range []int64{0, ofOtherRepo.ID} {
		path := apiV1RepoPath(repo, "/milestones/"+apiV1Itoa(id))
		status, body := api.do(http.MethodGet, path, nil)
		assert.Equal(t, http.StatusNotFound, status, "milestone %d, response: %s", id, body)
		status, body = api.do(http.MethodPatch, path, map[string]any{"state": "closed"})
		assert.Equal(t, http.StatusNotFound, status, "milestone %d, response: %s", id, body)
	}
	status, body := api.do(http.MethodDelete, apiV1RepoPath(repo, "/milestones/"+apiV1Itoa(ofOtherRepo.ID)), nil)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	var stillThere apiV1Milestone
	api.doJSON(http.MethodGet, apiV1RepoPath(otherRepo, "/milestones/"+apiV1Itoa(ofOtherRepo.ID)), nil, http.StatusOK, &stillThere)
	assert.Equal(t, ofOtherRepo, stillThere, "milestone of another repository changed")

	closedAfter := time.Now().Truncate(time.Second)
	var edited apiV1Milestone
	api.doJSON(http.MethodPatch, apiV1RepoPath(repo, "/milestones/"+apiV1Itoa(created.ID)), map[string]any{
		"title":       "v1.0",
		"description": "Edited",
		"state":       "closed",
	}, http.StatusOK, &edited)
	assert.Equal(t, "v1.0", edited.Title)
	assert.Equal(t, "closed", edited.State)

	var got apiV1Milestone
	api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/milestones/"+apiV1Itoa(created.ID)), nil, http.StatusOK, &got)
	assert.Equal(t, "v1.0", got.Title)
	assert.Equal(t, "Edited", got.Description)
	assert.Equal(t, "closed", got.State)
	require.NotNil(t, got.Closed)
	assert.WithinRange(t, *got.Closed, closedAfter, time.Now(), "closed date")
	assert.Equal(t, [2]int{1, 1}, countsOf())

	status, body = apiV1Client(t, seedUser(t, CreateUserOptions{})).do(http.MethodDelete, apiV1RepoPath(repo, "/milestones/"+apiV1Itoa(created.ID)), nil)
	assert.Equal(t, http.StatusForbidden, status, "deleted by another user, response: %s", body)

	status, body = api.do(http.MethodDelete, apiV1RepoPath(repo, "/milestones/"+apiV1Itoa(created.ID)), nil)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	status, body = api.do(http.MethodGet, apiV1RepoPath(repo, "/milestones/"+apiV1Itoa(created.ID)), nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	assert.Equal(t, [2]int{0, 0}, countsOf())
}
