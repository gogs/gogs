// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repo

import (
	"net/http"
	"time"

	api "github.com/gogs/go-gogs-client"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
)

// ListIssueComments list comments on an issue.
func ListIssueComments(c *context.APIContext) {
	// Initialize a variable to hold the "since" time
	var since time.Time
	// Check if the "since" query parameter is provided
	if len(c.Query("since")) > 0 {
		// Attempt to parse the "since" value as a time in RFC3339 format
		var err error
		since, err = time.Parse(time.RFC3339, c.Query("since"))
		if err != nil {
			c.ErrorStatus(http.StatusUnprocessableEntity, err)
			return
		}
	}

	// Retrieve the raw issue by its index
	issue, err := database.GetRawIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	// If an error occurs, return an error response
	if err != nil {
		c.Error(err, "get raw issue by index")
		return
	}

	// Initialize a boolean variable to determine the sort order
	var isAsc bool = true
	// Check if the "is_asc" query parameter is set to "false"
	if c.Query("is_asc") == "false" {
		// If so, set the sort order to descending
		isAsc = false
	}

	// Retrieve comments for the issue since a given time
	comments, err := database.GetCommentsByIssueIDSince(issue.ID, since.Unix(), isAsc)
	// If an error occurs, return an error response
	if err != nil {
		c.Error(err, "get comments by issue ID")
		return
	}

	// Create a slice of API comments to hold the formatted comments
	apiComments := make([]*api.Comment, len(comments))
	// Iterate over the comments and format them for the API
	for i := range comments {
		apiComments[i] = comments[i].APIFormat()
	}
	// Return the formatted comments as a JSON response
	c.JSONSuccess(&apiComments)
}

// ListRepoIssueComments list comments for a given repo.
func ListRepoIssueComments(c *context.APIContext) {
	var since time.Time
	// Check if the "since" query parameter is provided
	if len(c.Query("since")) > 0 {
		// Attempt to parse the "since" value as a time in RFC3339 format
		var err error
		since, err = time.Parse(time.RFC3339, c.Query("since"))
		if err != nil {
			c.ErrorStatus(http.StatusUnprocessableEntity, err)
			return
		}
	}

	// Initialize a boolean variable to determine the sort order
	var isAsc bool = true
	// Check if the "is_asc" query parameter is set to "false"
	if c.Query("is_asc") == "false" {
		// If so, set the sort order to descending
		isAsc = false
	}

	// Retrieve comments for the repository since a given time
	comments, err := database.GetCommentsByRepoIDSince(c.Repo.Repository.ID, since.Unix(), isAsc)
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

	c.JSON(http.StatusCreated, comment.APIFormat())
}

func EditIssueComment(c *context.APIContext, form api.EditIssueCommentOption) {
	comment, err := database.GetCommentByID(c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrError(err, "get comment by ID")
		return
	}

	if c.User.ID != comment.PosterID && !c.Repo.IsAdmin() {
		c.Status(http.StatusForbidden)
		return
	} else if comment.Type != database.COMMENT_TYPE_COMMENT {
		c.NoContent()
		return
	}

	oldContent := comment.Content
	comment.Content = form.Body
	if err := database.UpdateComment(c.User, comment, oldContent); err != nil {
		c.Error(err, "update comment")
		return
	}
	c.JSONSuccess(comment.APIFormat())
}

func DeleteIssueComment(c *context.APIContext) {
	comment, err := database.GetCommentByID(c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrError(err, "get comment by ID")
		return
	}

	if c.User.ID != comment.PosterID && !c.Repo.IsAdmin() {
		c.Status(http.StatusForbidden)
		return
	} else if comment.Type != database.COMMENT_TYPE_COMMENT {
		c.NoContent()
		return
	}

	if err = database.DeleteCommentByID(c.User, comment.ID); err != nil {
		c.Error(err, "delete comment by ID")
		return
	}
	c.NoContent()
}
