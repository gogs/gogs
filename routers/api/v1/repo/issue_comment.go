// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repo

import (
	"time"

	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/context"
	"github.com/gogits/gogs/modules/log"
)

const (
	ISO8601Format = "2006-01-02T15:04:05Z"
)

// ListIssueComments list comments on an issue
func ListIssueComments(ctx *context.APIContext) {
	var since time.Time
	var withSince bool

	// we get the issue instead of comments directly
	// because to get comments we need to gets issue first,
	// and there is already comments in the issue
	issue, err := models.GetIssueByIndex(ctx.Repo.Repository.ID, ctx.ParamsInt64(":index"))
	if err != nil {
		ctx.Error(500, "Comments", err)
		return
	}

	// parse `since`, by default we don't use `since`
	if len(ctx.Query("since")) > 0 {
		var err error
		since, err = time.Parse(ISO8601Format, ctx.Query("since"))
		if err == nil {
			withSince = true
		}
	}

	apiComments := []*api.Comment{}
	for _, comment := range issue.Comments {
		if withSince && !comment.Created.After(since) {
			continue
		}
		apiComments = append(apiComments, comment.APIFormat())
	}

	ctx.JSON(200, &apiComments)
}

// CreateIssueComment create comment on an issue
func CreateIssueComment(ctx *context.APIContext, form api.CreateIssueCommentOption) {
	// check issue
	issue, err := models.GetIssueByIndex(ctx.Repo.Repository.ID, ctx.ParamsInt64(":index"))
	if err != nil {
		ctx.Error(500, "Comments", err)
		return
	}

	comment := &models.Comment{
		Content: form.Body,
	}

	if len(form.Body) == 0 {
		ctx.Handle(400, "CreateIssueComment:empty content", err)
		return
	}

	// create comment
	comment, err = models.CreateIssueComment(ctx.User, ctx.Repo.Repository, issue, form.Body, []string{})
	if err != nil {
		ctx.Handle(500, "CreateIssueComment", err)
		return
	}

	log.Trace("Comment created: %d/%d/%d", ctx.Repo.Repository.ID, issue.ID, comment.ID)

	// Refetch from database to assign some automatic values
	comment, err = models.GetCommentByID(comment.ID)
	if err != nil {
		log.Info("Failed to refetch comment:%v", err)
	}
	ctx.JSON(201, comment.APIFormat())
}

// EditIssueComment edits an issue comment
func EditIssueComment(ctx *context.APIContext, form api.EditIssueCommentOption) {
	comment, err := models.GetCommentByID(ctx.ParamsInt64(":id"))
	if err != nil {
		if models.IsErrCommentNotExist(err) {
			ctx.Error(404, "GetCommentByID", err)
		} else {
			ctx.Handle(500, "GetCommentByID", err)
		}
		return
	}

	if !ctx.IsSigned || (ctx.User.ID != comment.PosterID && !ctx.Repo.IsAdmin()) {
		ctx.Error(403, "edit comment", err)
		return
	} else if comment.Type != models.COMMENT_TYPE_COMMENT {
		ctx.Error(204, "edit comment", err)
		return
	}

	comment.Content = form.Body
	if len(comment.Content) == 0 {
		ctx.JSON(200, map[string]interface{}{
			"content": "",
		})
		return
	}

	if err := models.UpdateComment(comment); err != nil {
		ctx.Handle(500, "UpdateComment", err)
		return
	}
	ctx.JSON(200, comment.APIFormat())
}
