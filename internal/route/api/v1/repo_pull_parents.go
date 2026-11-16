package v1

import (
	"net/http"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/database"
)

type pullParentsResponse struct {
	// The numbers of the pull requests the pull request depends on.
	Parents []int64 `json:"parents"`
}

// localPullRequest returns the open pull request with the given number whose
// head branch is in the repository of the context. It writes an error response
// with given status and returns nil when there is none.
func localPullRequest(c *apiContext, index int64, status int, keep map[int64]bool) *database.PullRequest {
	issue, err := database.GetIssueByIndex(c.Repo.Repository.ID, index)
	if err != nil {
		if database.IsErrIssueNotExist(err) {
			c.ErrorStatus(status, errors.Newf("pull request #%d does not exist", index))
		} else {
			c.Error(err, "get issue by index")
		}
		return nil
	}
	if !issue.IsPull || issue.PullRequest == nil {
		c.ErrorStatus(status, errors.Newf("#%d is not a pull request", index))
		return nil
	}

	pr := issue.PullRequest
	pr.Issue = issue
	if (pr.HasMerged || issue.IsClosed) && !keep[pr.ID] {
		c.ErrorStatus(status, errors.Newf("pull request #%d is not open", index))
		return nil
	} else if pr.HeadRepoID != c.Repo.Repository.ID {
		c.ErrorStatus(status, errors.Newf("pull request #%d is from another repository", index))
		return nil
	}
	return pr
}

func listPullParents(c *apiContext) {
	issue, err := database.GetIssueByIndex(c.Repo.Repository.ID, c.ParamInt64("index"))
	if err != nil {
		c.NotFoundOrError(err, "get issue by index")
		return
	} else if !issue.IsPull || issue.PullRequest == nil {
		c.NotFound()
		return
	}

	ids, err := database.Handle.PullRequestParents().ListParentIDs(c.Request().Context(), issue.PullRequest.ID)
	if err != nil {
		c.Error(err, "list parent IDs")
		return
	}
	resp := pullParentsResponse{Parents: make([]int64, 0, len(ids))}
	for _, id := range ids {
		parent, err := database.GetPullRequestByID(id)
		if err != nil {
			c.Error(err, "get parent")
			return
		}
		resp.Parents = append(resp.Parents, parent.Index)
	}
	c.JSONSuccess(&resp)
}

type setPullParentsRequest struct {
	// The numbers of the pull requests the pull request depends on, or none to
	// review it against its base branch again.
	Parents []int64 `json:"parents"`
}

func setPullParents(c *apiContext, form setPullParentsRequest) {
	pr := localPullRequest(c, c.ParamInt64("index"), http.StatusNotFound, nil)
	if pr == nil {
		return
	}
	// Like the web API, keep parents that merged or closed since they were
	// set, so the same list can be sent again.
	current, err := database.Handle.PullRequestParents().ListParentIDs(c.Request().Context(), pr.ID)
	if err != nil {
		c.Error(err, "list parent IDs")
		return
	}
	keep := make(map[int64]bool, len(current))
	for _, id := range current {
		keep[id] = true
	}

	parents := make([]*database.PullRequest, 0, len(form.Parents))
	for _, index := range form.Parents {
		parent := localPullRequest(c, index, http.StatusUnprocessableEntity, keep)
		if parent == nil {
			return
		}
		parents = append(parents, parent)
	}

	err = database.SetPullRequestParents(c.Request().Context(), pr, parents)
	if errors.Is(err, database.ErrPullRequestParentCycle) || errors.Is(err, database.ErrPullRequestParentsConflict) ||
		errors.Is(err, database.ErrPullRequestParentBaseMismatch) {
		c.ErrorStatus(http.StatusUnprocessableEntity, err)
		return
	} else if err != nil {
		c.Error(err, "set pull request parents")
		return
	}

	listPullParents(c)
}
