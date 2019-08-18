// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repo

import (
	"net/http"
	"time"

	api "github.com/gogs/go-gogs-client"

	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/pkg/context"
)

func ListIssueComments(c *context.APIContext) {
	var since time.Time
	if len(c.Query("since")) > 0 {
		var err error
		since, err = time.Parse(time.RFC3339, c.Query("since"))
		if err != nil {
			c.Error(http.StatusUnprocessableEntity, "", err)
			return
		}
	}

	// comments,err:=models.GetCommentsByIssueIDSince(, since)
	issue, err := models.GetRawIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		c.ServerError("GetRawIssueByIndex", err)
		return
	}

	comments, err := models.GetCommentsByIssueIDSince(issue.ID, since.Unix())
	if err != nil {
		c.ServerError("GetCommentsByIssueIDSince", err)
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
			c.Error(http.StatusUnprocessableEntity, "", err)
			return
		}
	}

	comments, err := models.GetCommentsByRepoIDSince(c.Repo.Repository.ID, since.Unix())
	if err != nil {
		c.ServerError("GetCommentsByRepoIDSince", err)
		return
	}

	apiComments := make([]*api.Comment, len(comments))
	for i := range comments {
		apiComments[i] = comments[i].APIFormat()
	}
	c.JSONSuccess(&apiComments)
}

func CreateIssueComment(c *context.APIContext, form api.CreateIssueCommentOption) {
	issue, err := models.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		c.ServerError("GetIssueByIndex", err)
		return
	}

	comment, err := models.CreateIssueComment(c.User, c.Repo.Repository, issue, form.Body, nil)
	if err != nil {
		c.ServerError("CreateIssueComment", err)
		return
	}

	c.JSON(http.StatusCreated, comment.APIFormat())
}

func EditIssueComment(c *context.APIContext, form api.EditIssueCommentOption) {
	comment, err := models.GetCommentByID(c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrServerError("GetCommentByID", models.IsErrCommentNotExist, err)
		return
	}

	if c.User.ID != comment.PosterID && !c.Repo.IsAdmin() {
		c.Status(http.StatusForbidden)
		return
	} else if comment.Type != models.COMMENT_TYPE_COMMENT {
		c.NoContent()
		return
	}

	oldContent := comment.Content
	comment.Content = form.Body
	if err := models.UpdateComment(c.User, comment, oldContent); err != nil {
		c.ServerError("UpdateComment", err)
		return
	}
	c.JSONSuccess(comment.APIFormat())
}

func DeleteIssueComment(c *context.APIContext) {
	comment, err := models.GetCommentByID(c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrServerError("GetCommentByID", models.IsErrCommentNotExist, err)
		return
	}

	if c.User.ID != comment.PosterID && !c.Repo.IsAdmin() {
		c.Status(http.StatusForbidden)
		return
	} else if comment.Type != models.COMMENT_TYPE_COMMENT {
		c.NoContent()
		return
	}

	if err = models.DeleteCommentByID(c.User, comment.ID); err != nil {
		c.ServerError("DeleteCommentByID", err)
		return
	}
	c.NoContent()
}
