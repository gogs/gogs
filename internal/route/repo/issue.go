package repo

import (
	"fmt"
	"net/url"
	"slices"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
)

func MustEnableIssues(c *context.Context) {
	if !c.Repo.Repository.EnableIssues {
		c.NotFound()
		return
	}

	if c.Repo.Repository.EnableExternalTracker {
		c.Redirect(c.Repo.Repository.ExternalTrackerURL)
		return
	}
}

func MustAllowPulls(c *context.Context) {
	if !c.Repo.Repository.AllowsPulls() {
		c.NotFound()
		return
	}

	// User can send pull request if owns a forked repository.
	if c.IsLogged && database.Handle.Repositories().HasForkedBy(c.Req.Context(), c.Repo.Repository.ID, c.User.ID) {
		c.Repo.PullRequest.Allowed = true
		c.Repo.PullRequest.HeadInfo = c.User.Name + ":" + c.Repo.BranchName
	}
}

// ServeIssueList serves the issue or pull request list of a repository. Like
// the server-rendered page did, it sends signed-out visitors to sign in for the
// lists about the viewer.
func ServeIssueList(c *context.Context) {
	if slices.Contains([]string{"assigned", "created_by", "mentioned"}, c.Query("type")) && !c.IsLogged {
		c.RawRedirect(conf.Server.Subpath + "/user/sign-in?redirect_to=" + url.QueryEscape(conf.Server.Subpath+c.Req.URL.RequestURI()))
		return
	}
	c.ServeWeb()
}

// ServeIssue serves the page of an issue, and sends pull requests to their
// own page.
func ServeIssue(c *context.Context) {
	index := c.ParamsInt64(":index")
	if index <= 0 {
		c.NotFound()
		return
	}
	issue, err := database.GetIssueByIndex(c.Repo.Repository.ID, index)
	if err != nil {
		c.NotFoundOrError(err, "get issue by index")
		return
	}
	if issue.IsPull {
		c.RawRedirect(c.Repo.MakeURL(fmt.Sprintf("pulls/%d", issue.Index)))
		return
	}
	MustEnableIssues(c)
	if c.Written() {
		return
	}
	c.ServeWeb()
}

// ServePull serves the page of a pull request, and sends issues to their own
// page.
func ServePull(c *context.Context) {
	index := c.ParamsInt64(":index")
	if index <= 0 {
		c.NotFound()
		return
	}
	issue, err := database.GetIssueByIndex(c.Repo.Repository.ID, index)
	if err != nil {
		c.NotFoundOrError(err, "get issue by index")
		return
	}
	if !issue.IsPull {
		c.RawRedirect(c.Repo.MakeURL(fmt.Sprintf("issues/%d", issue.Index)))
		return
	}
	MustAllowPulls(c)
	if c.Written() {
		return
	}
	c.ServeWeb()
}
