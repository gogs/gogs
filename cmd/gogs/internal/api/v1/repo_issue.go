package v1

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/api/v1types"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
)

func queryIssues(c *apiContext, opts *database.IssuesOptions) {
	issues, err := database.Handle.Issues().List(c.Request().Context(), opts)
	if err != nil {
		c.Error(err, "list issues")
		return
	}

	count, err := database.Handle.Issues().Count(c.Request().Context(), opts)
	if err != nil {
		c.Error(err, "count issues")
		return
	}

	apiIssues := make([]*v1types.Issue, len(issues))
	for i := range issues {
		apiIssues[i] = toIssue(issues[i])
	}

	c.SetLinkHeader(int(count), conf.UI.IssuePagingNum)
	c.JSONSuccess(&apiIssues)
}

func listUserIssues(c *apiContext) {
	opts := database.IssuesOptions{
		AssigneeID: c.User.ID,
		Page:       c.queryPage(conf.UI.IssuePagingNum),
		IsClosed:   v1types.IssueStateType(c.Query("state")) == v1types.IssueStateClosed,
	}

	queryIssues(c, &opts)
}

func listIssues(c *apiContext) {
	opts := database.IssuesOptions{
		RepoID:   c.Repo.Repository.ID,
		Page:     c.queryPage(conf.UI.IssuePagingNum),
		IsClosed: v1types.IssueStateType(c.Query("state")) == v1types.IssueStateClosed,
	}

	queryIssues(c, &opts)
}

func getIssue(c *apiContext) {
	issue, err := database.Handle.Issues().GetByIndex(c.Request().Context(), c.Repo.Repository.ID, c.ParamInt64("index"))
	if err != nil {
		c.NotFoundOrError(err, "get issue by index")
		return
	}
	c.JSONSuccess(toIssue(issue))
}

type createIssueRequest struct {
	Title     string  `json:"title" binding:"Required"`
	Body      string  `json:"body"`
	Assignee  string  `json:"assignee"`
	Milestone int64   `json:"milestone"`
	Labels    []int64 `json:"labels"`
	Closed    bool    `json:"closed"`
}

// isAssignable returns true if issues of the repository can be assigned to the
// user, the same users as in the web interface.
func isAssignable(ctx context.Context, repo *database.Repository, u *database.User) (bool, error) {
	assignees, err := database.Handle.Repositories().ListAssignees(ctx, repo)
	if err != nil {
		return false, errors.Wrap(err, "list assignees")
	}
	return slices.ContainsFunc(assignees, func(a *database.User) bool { return a.ID == u.ID }), nil
}

func createIssue(c *apiContext, form createIssueRequest) {
	issue := &database.Issue{
		RepoID:   c.Repo.Repository.ID,
		Title:    form.Title,
		PosterID: c.User.ID,
		Poster:   c.User,
		Content:  form.Body,
	}

	if c.Repo.IsWriter() {
		if len(form.Assignee) > 0 {
			assignee, err := database.Handle.Users().GetByUsername(c.Request().Context(), form.Assignee)
			if err != nil {
				if database.IsErrUserNotExist(err) {
					c.ErrorStatus(http.StatusUnprocessableEntity, errors.Newf("assignee does not exist: [name: %s]", form.Assignee))
				} else {
					c.Error(err, "get user by name")
				}
				return
			}
			assignable, err := isAssignable(c.Request().Context(), c.Repo.Repository, assignee)
			if err != nil {
				c.Error(err, "check assignee")
				return
			} else if !assignable {
				c.ErrorStatus(http.StatusUnprocessableEntity, errors.Newf("assignee cannot be assigned issues of the repository: [name: %s]", form.Assignee))
				return
			}
			issue.AssigneeID = assignee.ID
		}
		issue.MilestoneID = form.Milestone
	} else {
		form.Labels = nil
	}

	if err := database.Handle.Issues().Create(c.Request().Context(), c.Logger, c.Repo.Repository, issue, form.Labels, nil); err != nil {
		c.Error(err, "new issue")
		return
	}

	if form.Closed {
		if err := database.Handle.Issues().ChangeStatus(c.Request().Context(), c.Logger, c.User, c.Repo.Repository, issue, true); err != nil {
			c.Error(err, "change status to closed")
			return
		}
	}

	// Refetch from database to assign some automatic values
	var err error
	issue, err = database.Handle.Issues().GetByID(c.Request().Context(), issue.ID)
	if err != nil {
		c.Error(err, "get issue by ID")
		return
	}
	c.JSON(http.StatusCreated, toIssue(issue))
}

type editIssueRequest struct {
	Title     string  `json:"title"`
	Body      *string `json:"body"`
	Assignee  *string `json:"assignee"`
	Milestone *int64  `json:"milestone"`
	State     *string `json:"state"`
}

func editIssue(c *apiContext, form editIssueRequest) {
	issue, err := database.Handle.Issues().GetByIndex(c.Request().Context(), c.Repo.Repository.ID, c.ParamInt64("index"))
	if err != nil {
		c.NotFoundOrError(err, "get issue by index")
		return
	}

	if !issue.IsPoster(c.User.ID) && !c.Repo.IsWriter() {
		c.Status(http.StatusForbidden)
		return
	}

	// Stopping midway, e.g., when the client disconnects, would leave a partial
	// change behind.
	ctx := context.WithoutCancel(c.Request().Context())
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
			assignee, err := database.Handle.Users().GetByUsername(ctx, *form.Assignee)
			if err != nil {
				if database.IsErrUserNotExist(err) {
					c.ErrorStatus(http.StatusUnprocessableEntity, errors.Newf("assignee does not exist: [name: %s]", *form.Assignee))
				} else {
					c.Error(err, "get user by name")
				}
				return
			}
			assignable, err := isAssignable(c.Request().Context(), c.Repo.Repository, assignee)
			if err != nil {
				c.Error(err, "check assignee")
				return
			} else if !assignable {
				c.ErrorStatus(http.StatusUnprocessableEntity, errors.Newf("assignee cannot be assigned issues of the repository: [name: %s]", *form.Assignee))
				return
			}
			issue.AssigneeID = assignee.ID
		}

		if err = database.Handle.Issues().UpdateAssignee(ctx, issue); err != nil {
			c.Error(err, "update issue user by assignee")
			return
		}
	}
	if c.Repo.IsWriter() && form.Milestone != nil &&
		issue.MilestoneID != *form.Milestone {
		issue.MilestoneID = *form.Milestone
		if err = database.Handle.Milestones().AssignIssue(ctx, c.Logger, c.User, issue); err != nil {
			c.Error(err, "change milestone assign")
			return
		}
	}

	if err = database.Handle.Issues().Update(ctx, issue); err != nil {
		c.Error(err, "update issue")
		return
	}
	if form.State != nil {
		if err = database.Handle.Issues().ChangeStatus(ctx, c.Logger, c.User, c.Repo.Repository, issue, v1types.IssueStateClosed == v1types.IssueStateType(*form.State)); err != nil {
			c.Error(err, "change status")
			return
		}
	}

	// Refetch from database to assign some automatic values
	issue, err = database.Handle.Issues().GetByID(ctx, issue.ID)
	if err != nil {
		c.Error(err, "get issue by ID")
		return
	}
	c.JSON(http.StatusCreated, toIssue(issue))
}
