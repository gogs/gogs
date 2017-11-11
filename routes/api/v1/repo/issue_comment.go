// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repo

import (
	"time"

	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/pkg/context"
)

func ListIssueComments(c *context.APIContext) {
	var since time.Time
	if len(c.Query("since")) > 0 {
		var err error
		since, err = time.Parse(time.RFC3339, c.Query("since"))
		if err != nil {
			c.Error(422, "", err)
			return
		}
	}

	// comments,err:=models.GetCommentsByIssueIDSince(, since)
	issue, err := models.GetRawIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		c.Error(500, "GetRawIssueByIndex", err)
		return
	}

	comments, err := models.GetCommentsByIssueIDSince(issue.ID, since.Unix())
	if err != nil {
		c.Error(500, "GetCommentsByIssueIDSince", err)
		return
	}

	apiComments := make([]*api.Comment, len(comments))
	for i := range comments {
		apiComments[i] = comments[i].APIFormat()
	}
	c.JSON(200, &apiComments)
}

func ListRepoIssueComments(c *context.APIContext) {
	var since time.Time
	if len(c.Query("since")) > 0 {
		var err error
		since, err = time.Parse(time.RFC3339, c.Query("since"))
		if err != nil {
			c.Error(422, "", err)
			return
		}
	}

	comments, err := models.GetCommentsByRepoIDSince(c.Repo.Repository.ID, since.Unix())
	if err != nil {
		c.Error(500, "GetCommentsByRepoIDSince", err)
		return
	}

	apiComments := make([]*api.Comment, len(comments))
	for i := range comments {
		apiComments[i] = comments[i].APIFormat()
	}
	c.JSON(200, &apiComments)
}

func CreateIssueComment(c *context.APIContext, form api.CreateIssueCommentOption) {
	issue, err := models.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		c.Error(500, "GetIssueByIndex", err)
		return
	}

	comment, err := models.CreateIssueComment(c.User, c.Repo.Repository, issue, form.Body, nil)
	if err != nil {
		c.Error(500, "CreateIssueComment", err)
		return
	}

	c.JSON(201, comment.APIFormat())
}

func EditIssueComment(c *context.APIContext, form api.EditIssueCommentOption) {
	comment, err := models.GetCommentByID(c.ParamsInt64(":id"))
	if err != nil {
		if models.IsErrCommentNotExist(err) {
			c.Error(404, "GetCommentByID", err)
		} else {
			c.Error(500, "GetCommentByID", err)
		}
		return
	}

	if c.User.ID != comment.PosterID && !c.Repo.IsAdmin() {
		c.Status(403)
		return
	} else if comment.Type != models.COMMENT_TYPE_COMMENT {
		c.Status(204)
		return
	}

	oldContent := comment.Content
	comment.Content = form.Body
	if err := models.UpdateComment(c.User, comment, oldContent); err != nil {
		c.Error(500, "UpdateComment", err)
		return
	}
	c.JSON(200, comment.APIFormat())
}

func DeleteIssueComment(c *context.APIContext) {
	comment, err := models.GetCommentByID(c.ParamsInt64(":id"))
	if err != nil {
		if models.IsErrCommentNotExist(err) {
			c.Error(404, "GetCommentByID", err)
		} else {
			c.Error(500, "GetCommentByID", err)
		}
		return
	}

	if c.User.ID != comment.PosterID && !c.Repo.IsAdmin() {
		c.Status(403)
		return
	} else if comment.Type != models.COMMENT_TYPE_COMMENT {
		c.Status(204)
		return
	}

	if err = models.DeleteCommentByID(c.User, comment.ID); err != nil {
		c.Error(500, "DeleteCommentByID", err)
		return
	}
	c.Status(204)
}
