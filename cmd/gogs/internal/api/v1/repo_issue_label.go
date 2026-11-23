package v1

import (
	"net/http"

	"gogs.io/gogs/internal/api/v1types"
	"gogs.io/gogs/internal/database"
)

func listIssueLabels(c *apiContext) {
	issue, err := database.Handle.Issues().GetByIndex(c.Request().Context(), c.Repo.Repository.ID, c.ParamInt64("index"))
	if err != nil {
		c.NotFoundOrError(err, "get issue by index")
		return
	}

	apiLabels := make([]*v1types.IssueLabel, len(issue.Labels))
	for i := range issue.Labels {
		apiLabels[i] = toIssueLabel(issue.Labels[i])
	}
	c.JSONSuccess(&apiLabels)
}

type issueLabelsRequest struct {
	Labels []int64 `json:"labels"`
}

func addIssueLabels(c *apiContext, links database.LinkOptions, form issueLabelsRequest) {
	issue, err := database.Handle.Issues().GetByIndex(c.Request().Context(), c.Repo.Repository.ID, c.ParamInt64("index"))
	if err != nil {
		c.NotFoundOrError(err, "get issue by index")
		return
	}

	labels, err := database.Handle.Labels().ListByIDs(c.Request().Context(), c.Repo.Repository.ID, form.Labels)
	if err != nil {
		c.Error(err, "get labels in repository by IDs")
		return
	}

	if err = database.Handle.Issues().AddLabels(c.Request().Context(), c.Logger, links, c.User, issue, labels...); err != nil {
		c.Error(err, "add labels")
		return
	}

	labels, err = database.Handle.Labels().ListByIssueID(c.Request().Context(), issue.ID)
	if err != nil {
		c.Error(err, "get labels by issue ID")
		return
	}

	apiLabels := make([]*v1types.IssueLabel, len(labels))
	for i := range labels {
		apiLabels[i] = toIssueLabel(issue.Labels[i])
	}
	c.JSONSuccess(&apiLabels)
}

func deleteIssueLabel(c *apiContext) {
	issue, err := database.Handle.Issues().GetByIndex(c.Request().Context(), c.Repo.Repository.ID, c.ParamInt64("index"))
	if err != nil {
		c.NotFoundOrError(err, "get issue by index")
		return
	}

	label, err := database.Handle.Labels().GetByID(c.Request().Context(), c.Repo.Repository.ID, c.ParamInt64("id"))
	if err != nil {
		if database.IsErrLabelNotExist(err) {
			c.ErrorStatus(http.StatusUnprocessableEntity, err)
		} else {
			c.Error(err, "get label of repository by ID")
		}
		return
	}

	if err := database.Handle.Labels().RemoveFromIssue(c.Request().Context(), issue, label); err != nil {
		c.Error(err, "delete issue label")
		return
	}

	c.NoContent()
}

func replaceIssueLabels(c *apiContext, form issueLabelsRequest) {
	issue, err := database.Handle.Issues().GetByIndex(c.Request().Context(), c.Repo.Repository.ID, c.ParamInt64("index"))
	if err != nil {
		c.NotFoundOrError(err, "get issue by index")
		return
	}

	labels, err := database.Handle.Labels().ListByIDs(c.Request().Context(), c.Repo.Repository.ID, form.Labels)
	if err != nil {
		c.Error(err, "get labels in repository by IDs")
		return
	}

	if err := database.Handle.Issues().ReplaceLabels(c.Request().Context(), issue, labels...); err != nil {
		c.Error(err, "replace labels")
		return
	}

	labels, err = database.Handle.Labels().ListByIssueID(c.Request().Context(), issue.ID)
	if err != nil {
		c.Error(err, "get labels by issue ID")
		return
	}

	apiLabels := make([]*v1types.IssueLabel, len(labels))
	for i := range labels {
		apiLabels[i] = toIssueLabel(issue.Labels[i])
	}
	c.JSONSuccess(&apiLabels)
}

func clearIssueLabels(c *apiContext, links database.LinkOptions) {
	issue, err := database.Handle.Issues().GetByIndex(c.Request().Context(), c.Repo.Repository.ID, c.ParamInt64("index"))
	if err != nil {
		c.NotFoundOrError(err, "get issue by index")
		return
	}

	if err := database.Handle.Issues().ClearLabels(c.Request().Context(), c.Logger, links, c.User, issue); err != nil {
		c.Error(err, "clear labels")
		return
	}

	c.NoContent()
}
