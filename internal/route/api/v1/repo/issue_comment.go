// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repo

import (
	"net/http"
	"time"

	api "github.com/gogs/go-gogs-client"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
)

func ListIssueComments(c *context.APIContext) {
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
	issue, err := db.GetRawIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		c.Error(err, "get raw issue by index")
		return
	}

	comments, err := db.GetCommentsByIssueIDSince(issue.ID, since.Unix())
	if err != nil {
		c.Error(err, "get comments by issue ID")
		return
	}

	apiComments := make([]*api.Comment, len(comments))
	for i := range comments {
		apiComments[i] = comments[i].APIFormat()
	}
	c.JSONSuccess(&apiComments)
}

func ListRepoIssueComments(c *context.APIContext) {
	var since time.Time
	if len(c.Query("since")) > 0 {
		var err error
		since, err = time.Parse(time.RFC3339, c.Query("since"))
		if err != nil {
			c.ErrorStatus(http.StatusUnprocessableEntity, err)
			return
		}
	}

	comments, err := db.GetCommentsByRepoIDSince(c.Repo.Repository.ID, since.Unix())
	if err != nil {
		c.Error(err, "get comments by repository ID")
		return
	}

	apiComments := make([]*api.Comment, len(comments))
	for i := range comments {
		apiComments[i] = comments[i].APIFormat()
	}
	c.JSONSuccess(&apiComments)
}

func CreateIssueComment(c *context.APIContext, form api.CreateIssueCommentOption) {
	issue, err := db.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		c.Error(err, "get issue by index")
		return
	}

	comment, err := db.CreateIssueComment(c.User, c.Repo.Repository, issue, form.Body, nil)
	if err != nil {
		c.Error(err, "create issue comment")
		return
	}

	c.JSON(http.StatusCreated, comment.APIFormat())
}

func EditIssueComment(c *context.APIContext, form api.EditIssueCommentOption) {
	comment, err := db.GetCommentByID(c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrError(err, "get comment by ID")
		return
	}

	if c.User.ID != comment.PosterID && !c.Repo.IsAdmin() {
		c.Status(http.StatusForbidden)
		return
	} else if comment.Type != db.COMMENT_TYPE_COMMENT {
		c.NoContent()
		return
	}

	oldContent := comment.Content
	comment.Content = form.Body
	if err := db.UpdateComment(c.User, comment, oldContent); err != nil {
		c.Error(err, "update comment")
		return
	}
	c.JSONSuccess(comment.APIFormat())
}

func DeleteIssueComment(c *context.APIContext) {
	comment, err := db.GetCommentByID(c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrError(err, "get comment by ID")
		return
	}

	if c.User.ID != comment.PosterID && !c.Repo.IsAdmin() {
		c.Status(http.StatusForbidden)
		return
	} else if comment.Type != db.COMMENT_TYPE_COMMENT {
		c.NoContent()
		return
	}

	if err = db.DeleteCommentByID(c.User, comment.ID); err != nil {
		c.Error(err, "delete comment by ID")
		return
	}
	c.NoContent()
}
