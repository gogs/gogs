package v1

import (
	"net/http"
	"time"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/route/api/v1/types"
)

func listIssueComments(c *context.APIContext) {
	var since time.Time
	if len(c.Query("since")) > 0 {
		var err error
		since, err = time.Parse(time.RFC3339, c.Query("since"))
		if err != nil {
			c.ErrorStatus(http.StatusUnprocessableEntity, err)
			return
		}
	}

	// comments,err:=db.GetCommentsByIssueIDSince(, since)
	issue, err := database.GetRawIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		c.Error(err, "get raw issue by index")
		return
	}

	comments, err := database.GetCommentsByIssueIDSince(issue.ID, since.Unix())
	if err != nil {
		c.Error(err, "get comments by issue ID")
		return
	}

	apiComments := make([]*types.IssueComment, len(comments))
	for i := range comments {
		apiComments[i] = toIssueComment(comments[i])
	}
	c.JSONSuccess(&apiComments)
}

func listRepoIssueComments(c *context.APIContext) {
	var since time.Time
	if len(c.Query("since")) > 0 {
		var err error
		since, err = time.Parse(time.RFC3339, c.Query("since"))
		if err != nil {
			c.ErrorStatus(http.StatusUnprocessableEntity, err)
			return
		}
	}

	comments, err := database.GetCommentsByRepoIDSince(c.Repo.Repository.ID, since.Unix())
	if err != nil {
		c.Error(err, "get comments by repository ID")
		return
	}

	apiComments := make([]*types.IssueComment, len(comments))
	for i := range comments {
		apiComments[i] = toIssueComment(comments[i])
	}
	c.JSONSuccess(&apiComments)
}

type createIssueCommentRequest struct {
	Body string `json:"body" binding:"Required"`
}

func createIssueComment(c *context.APIContext, form createIssueCommentRequest) {
	issue, err := database.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		c.Error(err, "get issue by index")
		return
	}

	comment, err := database.CreateIssueComment(c.User, c.Repo.Repository, issue, form.Body, nil)
	if err != nil {
		c.Error(err, "create issue comment")
		return
	}

	c.JSON(http.StatusCreated, toIssueComment(comment))
}

type editIssueCommentRequest struct {
	Body string `json:"body" binding:"Required"`
}

func editIssueComment(c *context.APIContext, form editIssueCommentRequest) {
	comment, err := database.GetCommentByID(c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrError(err, "get comment by ID")
		return
	}

	issue, err := database.GetIssueByID(comment.IssueID)
	if err != nil {
		c.NotFoundOrError(err, "get issue by ID")
		return
	}

	if issue.RepoID != c.Repo.Repository.ID {
		c.NotFound()
		return
	}

	if c.User.ID != comment.PosterID && !c.Repo.IsAdmin() {
		c.Status(http.StatusForbidden)
		return
	} else if comment.Type != database.CommentTypeComment {
		c.NoContent()
		return
	}

	oldContent := comment.Content
	comment.Content = form.Body
	if err := database.UpdateComment(c.User, comment, oldContent); err != nil {
		c.Error(err, "update comment")
		return
	}
	c.JSONSuccess(toIssueComment(comment))
}

func deleteIssueComment(c *context.APIContext) {
	comment, err := database.GetCommentByID(c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrError(err, "get comment by ID")
		return
	}

	issue, err := database.GetIssueByID(comment.IssueID)
	if err != nil {
		c.NotFoundOrError(err, "get issue by ID")
		return
	}

	if issue.RepoID != c.Repo.Repository.ID {
		c.NotFound()
		return
	}

	if c.User.ID != comment.PosterID && !c.Repo.IsAdmin() {
		c.Status(http.StatusForbidden)
		return
	} else if comment.Type != database.CommentTypeComment {
		c.NoContent()
		return
	}

	if err = database.DeleteCommentByID(c.User, comment.ID); err != nil {
		c.Error(err, "delete comment by ID")
		return
	}
	c.NoContent()
}
