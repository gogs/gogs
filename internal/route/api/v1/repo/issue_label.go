package repo

import (
	"net/http"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/route/api/v1/apitype"
	"gogs.io/gogs/internal/route/api/v1/convert"
)

type IssueLabelsRequest struct {
	Labels []int64 `json:"labels"`
}

func ListIssueLabels(c *context.APIContext) {
	issue, err := database.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		c.NotFoundOrError(err, "get issue by index")
		return
	}

	apiLabels := make([]*apitype.Label, len(issue.Labels))
	for i := range issue.Labels {
		apiLabels[i] = convert.ToLabel(issue.Labels[i])
	}
	c.JSONSuccess(&apiLabels)
}

func AddIssueLabels(c *context.APIContext, form IssueLabelsRequest) {
	issue, err := database.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		c.NotFoundOrError(err, "get issue by index")
		return
	}

	labels, err := database.GetLabelsInRepoByIDs(c.Repo.Repository.ID, form.Labels)
	if err != nil {
		c.Error(err, "get labels in repository by IDs")
		return
	}

	if err = issue.AddLabels(c.User, labels); err != nil {
		c.Error(err, "add labels")
		return
	}

	labels, err = database.GetLabelsByIssueID(issue.ID)
	if err != nil {
		c.Error(err, "get labels by issue ID")
		return
	}

	apiLabels := make([]*apitype.Label, len(labels))
	for i := range labels {
		apiLabels[i] = convert.ToLabel(issue.Labels[i])
	}
	c.JSONSuccess(&apiLabels)
}

func DeleteIssueLabel(c *context.APIContext) {
	issue, err := database.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		c.NotFoundOrError(err, "get issue by index")
		return
	}

	label, err := database.GetLabelOfRepoByID(c.Repo.Repository.ID, c.ParamsInt64(":id"))
	if err != nil {
		if database.IsErrLabelNotExist(err) {
			c.ErrorStatus(http.StatusUnprocessableEntity, err)
		} else {
			c.Error(err, "get label of repository by ID")
		}
		return
	}

	if err := database.DeleteIssueLabel(issue, label); err != nil {
		c.Error(err, "delete issue label")
		return
	}

	c.NoContent()
}

func ReplaceIssueLabels(c *context.APIContext, form IssueLabelsRequest) {
	issue, err := database.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		c.NotFoundOrError(err, "get issue by index")
		return
	}

	labels, err := database.GetLabelsInRepoByIDs(c.Repo.Repository.ID, form.Labels)
	if err != nil {
		c.Error(err, "get labels in repository by IDs")
		return
	}

	if err := issue.ReplaceLabels(labels); err != nil {
		c.Error(err, "replace labels")
		return
	}

	labels, err = database.GetLabelsByIssueID(issue.ID)
	if err != nil {
		c.Error(err, "get labels by issue ID")
		return
	}

	apiLabels := make([]*apitype.Label, len(labels))
	for i := range labels {
		apiLabels[i] = convert.ToLabel(issue.Labels[i])
	}
	c.JSONSuccess(&apiLabels)
}

func ClearIssueLabels(c *context.APIContext) {
	issue, err := database.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		c.NotFoundOrError(err, "get issue by index")
		return
	}

	if err := issue.ClearLabels(c.User); err != nil {
		c.Error(err, "clear labels")
		return
	}

	c.NoContent()
}
