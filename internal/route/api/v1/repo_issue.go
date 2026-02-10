package v1

import (
	"net/http"
	"strings"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/route/api/v1/types"
)

type CreateIssueRequest struct {
	Title     string  `json:"title" binding:"Required"`
	Body      string  `json:"body"`
	Assignee  string  `json:"assignee"`
	Milestone int64   `json:"milestone"`
	Labels    []int64 `json:"labels"`
	Closed    bool    `json:"closed"`
}

type EditIssueRequest struct {
	Title     string  `json:"title"`
	Body      *string `json:"body"`
	Assignee  *string `json:"assignee"`
	Milestone *int64  `json:"milestone"`
	State     *string `json:"state"`
}

func listIssues(c *context.APIContext, opts *database.IssuesOptions) {
	issues, err := database.Issues(opts)
	if err != nil {
		c.Error(err, "list issues")
		return
	}

	count, err := database.IssuesCount(opts)
	if err != nil {
		c.Error(err, "count issues")
		return
	}

	// FIXME: use IssueList to improve performance.
	apiIssues := make([]*types.Issue, len(issues))
	for i := range issues {
		if err = issues[i].LoadAttributes(); err != nil {
			c.Error(err, "load attributes")
			return
		}
		apiIssues[i] = ToIssue(issues[i])
	}

	c.SetLinkHeader(int(count), conf.UI.IssuePagingNum)
	c.JSONSuccess(&apiIssues)
}

func ListUserIssues(c *context.APIContext) {
	opts := database.IssuesOptions{
		AssigneeID: c.User.ID,
		Page:       c.QueryInt("page"),
		IsClosed:   types.IssueStateType(c.Query("state")) == types.IssueStateClosed,
	}

	listIssues(c, &opts)
}

func ListIssues(c *context.APIContext) {
	opts := database.IssuesOptions{
		RepoID:   c.Repo.Repository.ID,
		Page:     c.QueryInt("page"),
		IsClosed: types.IssueStateType(c.Query("state")) == types.IssueStateClosed,
	}

	listIssues(c, &opts)
}

func GetIssue(c *context.APIContext) {
	issue, err := database.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		c.NotFoundOrError(err, "get issue by index")
		return
	}
	c.JSONSuccess(ToIssue(issue))
}

func CreateIssue(c *context.APIContext, form CreateIssueRequest) {
	issue := &database.Issue{
		RepoID:   c.Repo.Repository.ID,
		Title:    form.Title,
		PosterID: c.User.ID,
		Poster:   c.User,
		Content:  form.Body,
	}

	if c.Repo.IsWriter() {
		if len(form.Assignee) > 0 {
			assignee, err := database.Handle.Users().GetByUsername(c.Req.Context(), form.Assignee)
			if err != nil {
				if database.IsErrUserNotExist(err) {
					c.ErrorStatus(http.StatusUnprocessableEntity, errors.Newf("assignee does not exist: [name: %s]", form.Assignee))
				} else {
					c.Error(err, "get user by name")
				}
				return
			}
			issue.AssigneeID = assignee.ID
		}
		issue.MilestoneID = form.Milestone
	} else {
		form.Labels = nil
	}

	if err := database.NewIssue(c.Repo.Repository, issue, form.Labels, nil); err != nil {
		c.Error(err, "new issue")
		return
	}

	if form.Closed {
		if err := issue.ChangeStatus(c.User, c.Repo.Repository, true); err != nil {
			c.Error(err, "change status to closed")
			return
		}
	}

	// Refetch from database to assign some automatic values
	var err error
	issue, err = database.GetIssueByID(issue.ID)
	if err != nil {
		c.Error(err, "get issue by ID")
		return
	}
	c.JSON(http.StatusCreated, ToIssue(issue))
}

func EditIssue(c *context.APIContext, form EditIssueRequest) {
	issue, err := database.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		c.NotFoundOrError(err, "get issue by index")
		return
	}

	if !issue.IsPoster(c.User.ID) && !c.Repo.IsWriter() {
		c.Status(http.StatusForbidden)
		return
	}

	if len(form.Title) > 0 {
		issue.Title = form.Title
	}
	if form.Body != nil {
		issue.Content = *form.Body
	}

	if c.Repo.IsWriter() && form.Assignee != nil &&
		(issue.Assignee == nil || issue.Assignee.LowerName != strings.ToLower(*form.Assignee)) {
		if *form.Assignee == "" {
			issue.AssigneeID = 0
		} else {
			assignee, err := database.Handle.Users().GetByUsername(c.Req.Context(), *form.Assignee)
			if err != nil {
				if database.IsErrUserNotExist(err) {
					c.ErrorStatus(http.StatusUnprocessableEntity, errors.Newf("assignee does not exist: [name: %s]", *form.Assignee))
				} else {
					c.Error(err, "get user by name")
				}
				return
			}
			issue.AssigneeID = assignee.ID
		}

		if err = database.UpdateIssueUserByAssignee(issue); err != nil {
			c.Error(err, "update issue user by assignee")
			return
		}
	}
	if c.Repo.IsWriter() && form.Milestone != nil &&
		issue.MilestoneID != *form.Milestone {
		oldMilestoneID := issue.MilestoneID
		issue.MilestoneID = *form.Milestone
		if err = database.ChangeMilestoneAssign(c.User, issue, oldMilestoneID); err != nil {
			c.Error(err, "change milestone assign")
			return
		}
	}

	if err = database.UpdateIssue(issue); err != nil {
		c.Error(err, "update issue")
		return
	}
	if form.State != nil {
		if err = issue.ChangeStatus(c.User, c.Repo.Repository, types.IssueStateClosed == types.IssueStateType(*form.State)); err != nil {
			c.Error(err, "change status")
			return
		}
	}

	// Refetch from database to assign some automatic values
	issue, err = database.GetIssueByID(issue.ID)
	if err != nil {
		c.Error(err, "get issue by ID")
		return
	}
	c.JSON(http.StatusCreated, ToIssue(issue))
}
