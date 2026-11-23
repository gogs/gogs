package integrationtests

import (
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/dbx"
)

// pullGit runs Git in the repository on disk and returns its trimmed output.
func pullGit(t *testing.T, repo *Repository, args ...string) (string, error) {
	t.Helper()

	cmd := exec.Command("git", append([]string{"-C", database.RepoPath(cfg.Repository.Root, repo.Owner, repo.Name)}, args...)...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func pullMustGit(t *testing.T, repo *Repository, args ...string) string {
	t.Helper()

	out, err := pullGit(t, repo, args...)
	require.NoError(t, err, "git %v: %s", args, out)
	return out
}

// pullCommitToNewBranch commits a new file to a new branch made from main,
// through the editor of the web app as the client.
func pullCommitToNewBranch(t *testing.T, c *client, repo *Repository, branch, path, content string) {
	t.Helper()

	status, body := c.do(http.MethodPost, issueAPIPath(repo, "editor", "files"), map[string]any{
		"branch":    "main",
		"newBranch": branch,
		"summary":   "Add " + path,
		"path":      path,
		"content":   content,
	})
	require.Equal(t, http.StatusOK, status, "response: %s", body)
}

// pullCreate opens a pull request from the branch into main as the client
// and returns its index.
func pullCreate(t *testing.T, c *client, repo *Repository, branch, title string) int64 {
	t.Helper()

	var created struct {
		Index int64 `json:"index"`
	}
	c.doJSON(http.MethodPost, issueAPIPath(repo, "pulls"), map[string]any{
		"title": title,
		"base":  "main",
		"head":  branch,
	}, http.StatusCreated, &created)
	require.Positive(t, created.Index)
	return created.Index
}

type pullView struct {
	Index    int64  `json:"index"`
	Title    string `json:"title"`
	IsClosed bool   `json:"isClosed"`
	Pull     struct {
		State                  string `json:"state"`
		HeadBranch             string `json:"headBranch"`
		BaseBranch             string `json:"baseBranch"`
		CommitCount            int    `json:"commitCount"`
		FileCount              int    `json:"fileCount"`
		ViewerCanMerge         bool   `json:"viewerCanMerge"`
		ViewerNotInAllowlist   bool   `json:"viewerNotInAllowlist"`
		ViewerCanDeleteBranch  bool   `json:"viewerCanDeleteBranch"`
		ViewerCanRestoreBranch bool   `json:"viewerCanRestoreBranch"`
		Merger                 *struct {
			Name string `json:"name"`
		} `json:"merger"`
	} `json:"pull"`
}

func pullGet(t *testing.T, c *client, repo *Repository, index int64) *pullView {
	t.Helper()

	var v pullView
	c.doJSON(http.MethodGet, issueAPIPath(repo, "pulls", index), nil, http.StatusOK, &v)
	return &v
}

// pullWaitMergeable waits for the conflict check of the pull request, which
// may run in the background.
func pullWaitMergeable(t *testing.T, c *client, repo *Repository, index int64) *pullView {
	t.Helper()

	var v *pullView
	require.Eventually(t, func() bool {
		v = pullGet(t, c, repo, index)
		return v.Pull.State != "checking"
	}, 30*time.Second, 200*time.Millisecond)
	require.Equal(t, "mergeable", v.Pull.State)
	return v
}

func TestPullLifecycle(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	ownerClient := authenticatedClient(t, owner)
	otherClient := authenticatedClient(t, other)
	anonymous := newClient(t)

	branch := UniqueName("feature-")
	pullCommitToNewBranch(t, ownerClient, repo, branch, "feature.txt", "Hello pull request\n")
	headCommit := pullMustGit(t, repo, "rev-parse", "refs/heads/"+branch)
	mainBefore := pullMustGit(t, repo, "rev-parse", "refs/heads/main")

	t.Run("new pull request form", func(t *testing.T) {
		var resp struct {
			BaseBranch string   `json:"baseBranch"`
			HeadBranch string   `json:"headBranch"`
			State      string   `json:"state"`
			Branches   []string `json:"branches"`
			Commits    []struct {
				SHA string `json:"sha"`
			} `json:"commits"`
			Diff *struct {
				Base string `json:"base"`
				Head string `json:"head"`
			} `json:"diff"`
		}
		ownerClient.doJSON(http.MethodGet, issueAPIPath(repo, "pulls", "new", "main..."+branch), nil, http.StatusOK, &resp)
		assert.Equal(t, "ready", resp.State)
		assert.Equal(t, "main", resp.BaseBranch)
		assert.Equal(t, branch, resp.HeadBranch)
		assert.ElementsMatch(t, []string{"main", branch}, resp.Branches)
		require.Len(t, resp.Commits, 1)
		assert.Equal(t, headCommit, resp.Commits[0].SHA)
		require.NotNil(t, resp.Diff)
		assert.Equal(t, mainBefore, resp.Diff.Base)
		assert.Equal(t, headCommit, resp.Diff.Head)

		ownerClient.doJSON(http.MethodGet, issueAPIPath(repo, "pulls", "new", "main...main"), nil, http.StatusOK, &resp)
		assert.Equal(t, "even", resp.State)

		// Only writers of the head repository can compare its branches.
		status, body := otherClient.do(http.MethodGet, issueAPIPath(repo, "pulls", "new", "main..."+branch), nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
		status, body = ownerClient.do(http.MethodGet, issueAPIPath(repo, "pulls", "new", "main...no-such-branch"), nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})

	status, body := otherClient.do(http.MethodPost, issueAPIPath(repo, "pulls"), map[string]any{"title": "Intruder", "base": "main", "head": branch})
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	status, body = ownerClient.do(http.MethodPost, issueAPIPath(repo, "pulls"), map[string]any{"title": "Nothing", "base": "main", "head": "main"})
	assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)

	title := UniqueName("Add feature ")
	index := pullCreate(t, ownerClient, repo, branch, title)
	issue := issueFromDatabase(t, repo, index)
	assert.True(t, issue.IsPull)
	assert.Equal(t, title, issue.Title)
	pr, err := database.Handle.PullRequests().GetByIssueID(t.Context(), issue.ID)
	require.NoError(t, err)
	assert.Equal(t, branch, pr.HeadBranch)
	assert.Equal(t, "main", pr.BaseBranch)
	assert.Equal(t, headCommit, pullMustGit(t, repo, "rev-parse", "refs/pull/"+strconv.FormatInt(index, 10)+"/head"))

	status, body = ownerClient.do(http.MethodPost, issueAPIPath(repo, "pulls"), map[string]any{"title": "Again", "base": "main", "head": branch})
	assert.Equal(t, http.StatusConflict, status, "response: %s", body)

	t.Run("list", func(t *testing.T) {
		var list struct {
			Items []struct {
				Index int64  `json:"index"`
				Title string `json:"title"`
			} `json:"items"`
			OpenCount         int64  `json:"openCount"`
			NewPullRequestURL string `json:"newPullRequestURL"`
		}
		ownerClient.doJSON(http.MethodGet, issueAPIPath(repo, "pulls"), nil, http.StatusOK, &list)
		require.Len(t, list.Items, 1)
		assert.Equal(t, index, list.Items[0].Index)
		assert.Equal(t, title, list.Items[0].Title)
		assert.Equal(t, int64(1), list.OpenCount)
		assert.Equal(t, "/"+repo.Owner+"/"+repo.Name+"/compare/main...main", list.NewPullRequestURL)

		var otherList struct {
			NewPullRequestURL string `json:"newPullRequestURL"`
		}
		otherClient.doJSON(http.MethodGet, issueAPIPath(repo, "pulls"), nil, http.StatusOK, &otherList)
		assert.Empty(t, otherList.NewPullRequestURL, "only writers and those who forked propose pull requests")

		// Pull requests are not issues.
		var issues struct {
			Items []any `json:"items"`
		}
		ownerClient.doJSON(http.MethodGet, issueAPIPath(repo, "issues"), nil, http.StatusOK, &issues)
		assert.Empty(t, issues.Items)
		status, body := ownerClient.do(http.MethodGet, issueAPIPath(repo, "issues", index), nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

		var suggestions struct {
			Items []struct {
				Index  int64 `json:"index"`
				IsPull bool  `json:"isPull"`
			} `json:"items"`
		}
		ownerClient.doJSON(http.MethodGet, issueAPIPath(repo, "suggestions", "issues")+"?q="+url.QueryEscape(title), nil, http.StatusOK, &suggestions)
		require.Len(t, suggestions.Items, 1)
		assert.Equal(t, index, suggestions.Items[0].Index)
		assert.True(t, suggestions.Items[0].IsPull)
	})

	t.Run("view", func(t *testing.T) {
		v := pullWaitMergeable(t, ownerClient, repo, index)
		assert.Equal(t, title, v.Title)
		assert.Equal(t, branch, v.Pull.HeadBranch)
		assert.Equal(t, "main", v.Pull.BaseBranch)
		assert.Equal(t, 1, v.Pull.CommitCount)
		assert.Equal(t, 1, v.Pull.FileCount)
		assert.True(t, v.Pull.ViewerCanMerge)
		assert.False(t, pullGet(t, anonymous, repo, index).Pull.ViewerCanMerge)

		var commits struct {
			Commits []struct {
				SHA         string `json:"sha"`
				SubjectHTML string `json:"subjectHTML"`
			} `json:"commits"`
		}
		anonymous.doJSON(http.MethodGet, issueAPIPath(repo, "pulls", index, "commits"), nil, http.StatusOK, &commits)
		require.Len(t, commits.Commits, 1)
		assert.Equal(t, headCommit, commits.Commits[0].SHA)
		assert.Contains(t, commits.Commits[0].SubjectHTML, "Add feature.txt")

		status, header, diff := anonymous.doRaw(http.MethodGet, issuePagePath(repo, "pulls", strconv.FormatInt(index, 10)+".diff"), "", nil)
		require.Equal(t, http.StatusOK, status, "response: %s", diff)
		assert.Contains(t, header.Get("Content-Type"), "text/plain")
		assert.Contains(t, string(diff), "diff --git a/feature.txt b/feature.txt")
		assert.Contains(t, string(diff), "+Hello pull request")

		status, body := anonymous.do(http.MethodGet, issueAPIPath(repo, "pulls", index+1000), nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})

	t.Run("comment on pull request", func(t *testing.T) {
		var created struct {
			ID int64 `json:"id"`
		}
		otherClient.doJSON(http.MethodPost, issueAPIPath(repo, "pulls", index, "comments"), map[string]any{"content": "Looks good"}, http.StatusCreated, &created)
		comment, err := database.Handle.Comments().GetByID(t.Context(), created.ID)
		require.NoError(t, err)
		assert.Equal(t, issue.ID, comment.IssueID)
	})

	t.Run("merge", func(t *testing.T) {
		status, body := otherClient.do(http.MethodPost, issueAPIPath(repo, "pulls", index, "merge"), map[string]any{})
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
		status, body = ownerClient.do(http.MethodPost, issueAPIPath(repo, "pulls", index, "merge"), map[string]any{"style": "rebase_before_merging"})
		assert.Equal(t, http.StatusUnprocessableEntity, status, "rebasing is not allowed by default, response: %s", body)
		_, err := pullGit(t, repo, "cat-file", "-e", "main:feature.txt")
		require.Error(t, err, "merged without permission")

		status, body = ownerClient.do(http.MethodPost, issueAPIPath(repo, "pulls", index, "merge"), map[string]any{
			"style": "create_merge_commit",
			"title": "Merge the feature",
		})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)

		assert.Equal(t, "Hello pull request", pullMustGit(t, repo, "show", "main:feature.txt"))
		assert.Equal(t, "Merge the feature", pullMustGit(t, repo, "log", "-1", "--format=%s", "main"))
		assert.Equal(t, mainBefore+" "+headCommit, pullMustGit(t, repo, "log", "-1", "--format=%P", "main"))

		pr, err := database.Handle.PullRequests().GetByIssueID(t.Context(), issue.ID)
		require.NoError(t, err)
		assert.True(t, pr.HasMerged)
		assert.Equal(t, owner.ID, pr.MergerID)
		v := pullGet(t, ownerClient, repo, index)
		assert.Equal(t, "merged", v.Pull.State)
		assert.True(t, v.IsClosed)
		require.NotNil(t, v.Pull.Merger)
		assert.Equal(t, owner.Name, v.Pull.Merger.Name)
		assert.True(t, v.Pull.ViewerCanDeleteBranch)

		status, body = ownerClient.do(http.MethodPost, issueAPIPath(repo, "pulls", index, "merge"), map[string]any{})
		assert.Equal(t, http.StatusNotFound, status, "merged twice, response: %s", body)
		status, body = ownerClient.do(http.MethodDelete, issueAPIPath(repo, "pulls", index, "closed"), map[string]any{})
		assert.Equal(t, http.StatusNotFound, status, "reopened a merged pull request, response: %s", body)
	})

	t.Run("delete and restore branch", func(t *testing.T) {
		path := issueAPIPath(repo, "pulls", index, "branch")
		status, body := otherClient.do(http.MethodDelete, path, map[string]any{})
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
		status, body = ownerClient.do(http.MethodPost, path, map[string]any{})
		assert.Equal(t, http.StatusNotFound, status, "restored a branch that exists, response: %s", body)

		status, body = ownerClient.do(http.MethodDelete, path, map[string]any{})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		_, err := pullGit(t, repo, "rev-parse", "--verify", "refs/heads/"+branch)
		require.Error(t, err, "branch not deleted")
		v := pullGet(t, ownerClient, repo, index)
		assert.False(t, v.Pull.ViewerCanDeleteBranch)
		assert.True(t, v.Pull.ViewerCanRestoreBranch)
		assert.False(t, pullGet(t, otherClient, repo, index).Pull.ViewerCanRestoreBranch)

		status, body = otherClient.do(http.MethodPost, path, map[string]any{})
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
		status, body = ownerClient.do(http.MethodPost, path, map[string]any{})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		assert.Equal(t, headCommit, pullMustGit(t, repo, "rev-parse", "refs/heads/"+branch))
	})
}

func TestPullCloseAndReopen(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	c := authenticatedClient(t, owner)

	branch := UniqueName("topic-")
	pullCommitToNewBranch(t, c, repo, branch, "topic.md", "# Topic\n")
	index := pullCreate(t, c, repo, branch, "Topic")
	path := issueAPIPath(repo, "pulls", index)

	status, body := c.do(http.MethodPatch, path, map[string]any{"title": "Renamed topic"})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.Equal(t, "Renamed topic", issueFromDatabase(t, repo, index).Title)

	status, body = c.do(http.MethodPut, path+"/closed", map[string]any{})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.True(t, issueFromDatabase(t, repo, index).IsClosed)
	assert.Equal(t, "closed", pullGet(t, c, repo, index).Pull.State)
	status, body = c.do(http.MethodPost, path+"/merge", map[string]any{})
	assert.Equal(t, http.StatusNotFound, status, "merged a closed pull request, response: %s", body)

	// A closed pull request does not keep another from opening for the same
	// branches, which then keeps the closed one from reopening.
	other := pullCreate(t, c, repo, branch, "Topic again")
	status, body = c.do(http.MethodDelete, path+"/closed", map[string]any{})
	assert.Equal(t, http.StatusConflict, status, "response: %s", body)

	status, body = c.do(http.MethodPut, issueAPIPath(repo, "pulls", other, "closed"), map[string]any{})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	status, body = c.do(http.MethodDelete, path+"/closed", map[string]any{})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.False(t, issueFromDatabase(t, repo, index).IsClosed)
	pullWaitMergeable(t, c, repo, index)
}

func TestPullMergeConcurrently(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	c := authenticatedClient(t, owner)

	branch := UniqueName("topic-")
	pullCommitToNewBranch(t, c, repo, branch, "topic.md", "# Topic\n")
	index := pullCreate(t, c, repo, branch, "Topic")
	pullWaitMergeable(t, c, repo, index)

	issue := issueFromDatabase(t, repo, index)
	merge := lockedRequest{http.MethodPost, issueAPIPath(repo, "pulls", index, "merge"), map[string]any{}}
	statuses := c.doBothWhileLocked("SELECT id FROM issue WHERE id = ? FOR UPDATE", issue.ID, merge, merge)
	assert.ElementsMatch(t, []int{http.StatusNoContent, http.StatusNotFound}, statuses)

	comments, err := database.Handle.Comments().ListByIssueID(t.Context(), issue.ID, 0)
	require.NoError(t, err)
	var closes int
	for _, comment := range comments {
		if comment.Type == database.CommentTypeClose {
			closes++
		}
	}
	assert.Equal(t, 1, closes, "pull request closed more than once")

	t.Run("while closing it", func(t *testing.T) {
		branch := UniqueName("other-")
		pullCommitToNewBranch(t, c, repo, branch, "other.md", "# Other\n")
		index := pullCreate(t, c, repo, branch, "Other")
		pullWaitMergeable(t, c, repo, index)

		// Whichever request runs second finds the pull request closed and does
		// nothing.
		issue := issueFromDatabase(t, repo, index)
		statuses := c.doBothWhileLocked("SELECT id FROM issue WHERE id = ? FOR UPDATE", issue.ID,
			lockedRequest{http.MethodPut, issueAPIPath(repo, "pulls", index, "closed"), map[string]any{}},
			lockedRequest{http.MethodPost, issueAPIPath(repo, "pulls", index, "merge"), map[string]any{}})
		assert.Equal(t, []int{http.StatusNoContent, http.StatusNotFound}, statuses)

		pr, err := database.Handle.PullRequests().GetByIssueID(t.Context(), issue.ID)
		require.NoError(t, err)
		assert.False(t, pr.HasMerged)
		comments, err := database.Handle.Comments().ListByIssueID(t.Context(), issue.ID, 0)
		require.NoError(t, err)
		var closes int
		for _, comment := range comments {
			if comment.Type == database.CommentTypeClose {
				closes++
			}
		}
		assert.Equal(t, 1, closes, "pull request closed more than once")
		got, err := database.Handle.Repositories().GetByID(t.Context(), repo.ID)
		require.NoError(t, err)
		assert.Equal(t, 2, got.NumClosedPulls)
	})
}

// A merge that fails to close the pull request leaves the base branch as it
// is, so that the pull request can be merged again.
func TestPullMergeDatabaseFailure(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	c := authenticatedClient(t, owner)

	branch := UniqueName("topic-")
	pullCommitToNewBranch(t, c, repo, branch, "topic.md", "# Topic\n")
	index := pullCreate(t, c, repo, branch, "Topic")
	pullWaitMergeable(t, c, repo, index)
	issue := issueFromDatabase(t, repo, index)
	baseCommit := pullMustGit(t, repo, "rev-parse", "main")

	db, err := dbx.OpenDB(cfg.Database, &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer func() { _ = sqlDB.Close() }()
	function := fmt.Sprintf("fail_close_comment_%d", issue.ID)
	require.NoError(t, db.Exec(fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger AS $$ BEGIN RAISE EXCEPTION 'injected failure'; END $$ LANGUAGE plpgsql`, function)).Error)
	require.NoError(t, db.Exec(fmt.Sprintf(`CREATE TRIGGER %[1]s BEFORE INSERT ON comment FOR EACH ROW WHEN (NEW.issue_id = %[2]d) EXECUTE FUNCTION %[1]s()`, function, issue.ID)).Error)
	dropTrigger := func() {
		_ = db.Exec(fmt.Sprintf("DROP TRIGGER IF EXISTS %[1]s ON comment; DROP FUNCTION IF EXISTS %[1]s()", function)).Error
	}
	defer dropTrigger()

	mergePath := issueAPIPath(repo, "pulls", index, "merge")
	status, body := c.do(http.MethodPost, mergePath, map[string]any{})
	require.Equal(t, http.StatusInternalServerError, status, "response: %s", body)
	assert.Equal(t, baseCommit, pullMustGit(t, repo, "rev-parse", "main"))
	assert.False(t, issueFromDatabase(t, repo, index).IsClosed)

	dropTrigger()
	pullWaitMergeable(t, c, repo, index)
	status, body = c.do(http.MethodPost, mergePath, map[string]any{})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.NotEqual(t, baseCommit, pullMustGit(t, repo, "rev-parse", "main"))
}

func TestPullPrivateRepository(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
	ownerClient := authenticatedClient(t, owner)
	otherClient := authenticatedClient(t, other)

	branch := UniqueName("secret-")
	pullCommitToNewBranch(t, ownerClient, repo, branch, "secret.txt", "secret\n")
	index := pullCreate(t, ownerClient, repo, branch, "Secret")
	diffPath := issuePagePath(repo, "pulls", strconv.FormatInt(index, 10)+".diff")

	status, _, diff := ownerClient.doRaw(http.MethodGet, diffPath, "", nil)
	require.Equal(t, http.StatusOK, status)
	assert.Contains(t, string(diff), "+secret")

	for name, c := range map[string]*client{"anonymous": newClient(t), "other": otherClient} {
		for _, path := range []string{
			issueAPIPath(repo, "pulls"),
			issueAPIPath(repo, "pulls", index),
			issueAPIPath(repo, "pulls", index, "commits"),
			diffPath,
		} {
			status, _, body := c.doRaw(http.MethodGet, path, "", nil)
			assert.Equal(t, http.StatusNotFound, status, "%s viewer, GET %s", name, path)
			assert.NotContains(t, string(body), "secret", "%s viewer, GET %s", name, path)
		}
	}
}

// pullSetPulls turns the pull requests of the repository on or off as the
// client.
func pullSetPulls(t *testing.T, c *client, repo *Repository, enable bool) {
	t.Helper()

	status, body := c.do(http.MethodPut, repoSettingsAPIPath(repo, "/settings/advanced"), map[string]any{
		"enableIssues":    true,
		"enablePulls":     enable,
		"pullsAllowMerge": true,
	})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
}

// pullNewURL returns where the pull request list of the repository sends the
// client to propose a pull request.
func pullNewURL(t *testing.T, c *client, repo *Repository) string {
	t.Helper()

	var list struct {
		NewPullRequestURL string `json:"newPullRequestURL"`
	}
	c.doJSON(http.MethodGet, issueAPIPath(repo, "pulls"), nil, http.StatusOK, &list)
	return list.NewPullRequestURL
}

type pullCompareView struct {
	State     string `json:"state"`
	Head      string `json:"head"`
	BaseRepos []struct {
		Owner         string `json:"owner"`
		Name          string `json:"name"`
		DefaultBranch string `json:"defaultBranch"`
	} `json:"baseRepos"`
}

// pullCompareFork compares the branches of the range in the repository as
// the client, and returns the comparison with its base repositories written
// as "<owner>/<name>@<default branch>".
func pullCompareFork(t *testing.T, c *client, repo *Repository, rng string) (*pullCompareView, []string) {
	t.Helper()

	var v pullCompareView
	c.doJSON(http.MethodGet, issueAPIPath(repo, "pulls", "new", rng), nil, http.StatusOK, &v)
	bases := make([]string, 0, len(v.BaseRepos))
	for _, b := range v.BaseRepos {
		bases = append(bases, b.Owner+"/"+b.Name+"@"+b.DefaultBranch)
	}
	return &v, bases
}

func TestPullFork(t *testing.T) {
	t.Parallel()

	alice := seedUser(t, CreateUserOptions{})
	bob := seedUser(t, CreateUserOptions{})
	base := seedRepository(t, alice.ID, CreateRepositoryOptions{AutoInit: true})
	aliceClient := authenticatedClient(t, alice)
	bobClient := authenticatedClient(t, bob)

	fork := repoVisibilityFork(t, bobClient, base, bob)
	branch := UniqueName("feature-")
	pullCommitToNewBranch(t, bobClient, fork, branch, "feature.txt", "Hello fork\n")
	baseLink := "/" + base.Owner + "/" + base.Name
	forkLink := "/" + fork.Owner + "/" + fork.Name
	baseEntry := base.Owner + "/" + base.Name + "@main"
	forkEntry := fork.Owner + "/" + fork.Name + "@main"

	t.Run("fork without pull requests", func(t *testing.T) {
		status, body := bobClient.do(http.MethodGet, issueAPIPath(fork, "pulls"), nil)
		require.Equal(t, http.StatusNotFound, status, "forks start without pull requests, response: %s", body)

		assert.Equal(t, baseLink+"/compare/main..."+bob.Name+":main", pullNewURL(t, bobClient, base))

		v, bases := pullCompareFork(t, bobClient, base, "main..."+bob.Name+":"+branch)
		assert.Equal(t, "ready", v.State)
		assert.Equal(t, []string{baseEntry}, bases)
	})

	pullSetPulls(t, bobClient, fork, true)

	t.Run("fork with pull requests", func(t *testing.T) {
		assert.Equal(t, forkLink+"/compare/main...main", pullNewURL(t, bobClient, fork))
		// Those who forked the base still propose from their fork.
		assert.Equal(t, baseLink+"/compare/main..."+bob.Name+":main", pullNewURL(t, bobClient, base))
		assert.Equal(t, baseLink+"/compare/main...main", pullNewURL(t, aliceClient, base))

		// Both sides of the comparison offer the same repositories to merge
		// into, the fork first.
		v, bases := pullCompareFork(t, bobClient, fork, "main..."+branch)
		assert.Equal(t, "ready", v.State)
		assert.Equal(t, branch, v.Head)
		assert.Equal(t, []string{forkEntry, baseEntry}, bases)
		v, bases = pullCompareFork(t, bobClient, base, "main..."+bob.Name+":"+branch)
		assert.Equal(t, "ready", v.State)
		assert.Equal(t, bob.Name+":"+branch, v.Head)
		assert.Equal(t, []string{forkEntry, baseEntry}, bases)

		// A pull request within the fork does not count as one into the base
		// from the same branch, and the other way around.
		inFork := pullCreate(t, bobClient, fork, branch, "Within the fork")
		v, _ = pullCompareFork(t, bobClient, base, "main..."+bob.Name+":"+branch)
		assert.Equal(t, "ready", v.State)

		var created struct {
			Index int64 `json:"index"`
		}
		bobClient.doJSON(http.MethodPost, issueAPIPath(base, "pulls"), map[string]any{
			"title": "Into the base",
			"base":  "main",
			"head":  bob.Name + ":" + branch,
		}, http.StatusCreated, &created)
		v, _ = pullCompareFork(t, bobClient, fork, "main..."+branch)
		assert.Equal(t, "exists", v.State)

		inForkPull, err := database.Handle.PullRequests().GetByIssueID(t.Context(), issueFromDatabase(t, fork, inFork).ID)
		require.NoError(t, err)
		assert.Equal(t, fork.ID, inForkPull.HeadRepoID)
		assert.Equal(t, fork.ID, inForkPull.BaseRepoID)
		assert.Equal(t, pullMustGit(t, fork, "rev-parse", "refs/heads/"+branch), pullMustGit(t, fork, "rev-parse", "refs/pull/"+strconv.FormatInt(inFork, 10)+"/head"))
		intoBasePull, err := database.Handle.PullRequests().GetByIssueID(t.Context(), issueFromDatabase(t, base, created.Index).ID)
		require.NoError(t, err)
		assert.Equal(t, fork.ID, intoBasePull.HeadRepoID)
		assert.Equal(t, base.ID, intoBasePull.BaseRepoID)
		assert.Equal(t, bob.Name, intoBasePull.HeadUserName)
	})

	t.Run("base repository without pull requests is not offered", func(t *testing.T) {
		pullSetPulls(t, aliceClient, base, false)
		defer pullSetPulls(t, aliceClient, base, true)

		_, bases := pullCompareFork(t, bobClient, fork, "main..."+branch)
		assert.Equal(t, []string{forkEntry}, bases)
	})

	t.Run("base repository the viewer cannot read is not offered", func(t *testing.T) {
		repoVisibilityPatch(t, aliceClient, base, true, false)
		defer repoVisibilityPatch(t, aliceClient, base, false, false)

		_, bases := pullCompareFork(t, bobClient, fork, "main..."+branch)
		assert.Equal(t, []string{forkEntry}, bases)
	})
}
