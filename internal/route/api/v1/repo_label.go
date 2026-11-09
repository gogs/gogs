package v1

import (
	"net/http"
	"strconv"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/route/api/v1/types"
)

func listLabels(c *apiContext) {
	labels, err := database.GetLabelsByRepoID(c.Repo.Repository.ID)
	if err != nil {
		c.Error(err, "get labels by repository ID")
		return
	}

	apiLabels := make([]*types.IssueLabel, len(labels))
	for i := range labels {
		apiLabels[i] = toIssueLabel(labels[i])
	}
	c.JSONSuccess(&apiLabels)
}

func getLabel(c *apiContext) {
	var label *database.Label
	var err error
	idStr := c.Param("id")
	if id, _ := strconv.ParseInt(idStr, 10, 64); id > 0 {
		label, err = database.GetLabelOfRepoByID(c.Repo.Repository.ID, id)
	} else {
		label, err = database.GetLabelOfRepoByName(c.Repo.Repository.ID, idStr)
	}
	if err != nil {
		c.NotFoundOrError(err, "get label")
		return
	}

	c.JSONSuccess(toIssueLabel(label))
}

type createLabelRequest struct {
	Name  string `json:"name" validate:"required"`
	Color string `json:"color" validate:"required,len=7"`
}

func createLabel(c *apiContext, form createLabelRequest) {
	label := &database.Label{
		Name:   form.Name,
		Color:  form.Color,
		RepoID: c.Repo.Repository.ID,
	}
	if err := database.NewLabels(label); err != nil {
		c.Error(err, "new labels")
		return
	}
	c.JSON(http.StatusCreated, toIssueLabel(label))
}

type editLabelRequest struct {
	Name  *string `json:"name"`
	Color *string `json:"color"`
}

func editLabel(c *apiContext, form editLabelRequest) {
	label, err := database.GetLabelOfRepoByID(c.Repo.Repository.ID, c.ParamInt64("id"))
	if err != nil {
		c.NotFoundOrError(err, "get label of repository by ID")
		return
	}

	if form.Name != nil {
		label.Name = *form.Name
	}
	if form.Color != nil {
		label.Color = *form.Color
	}
	if err := database.UpdateLabel(label); err != nil {
		c.Error(err, "update label")
		return
	}
	c.JSONSuccess(toIssueLabel(label))
}

func deleteLabel(c *apiContext) {
	if err := database.DeleteLabel(c.Repo.Repository.ID, c.ParamInt64("id")); err != nil {
		c.Error(err, "delete label")
		return
	}

	c.NoContent()
}
