package v1

import (
	"net/http"
	"strconv"

	"gogs.io/gogs/internal/api/v1types"
	"gogs.io/gogs/internal/database"
)

func listLabels(c *apiContext) {
	labels, err := database.Handle.Labels().ListByRepoID(c.Request().Context(), c.Repo.Repository.ID)
	if err != nil {
		c.Error(err, "get labels by repository ID")
		return
	}

	apiLabels := make([]*v1types.IssueLabel, len(labels))
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
		label, err = database.Handle.Labels().GetByID(c.Request().Context(), c.Repo.Repository.ID, id)
	} else {
		label, err = database.Handle.Labels().GetByName(c.Request().Context(), c.Repo.Repository.ID, idStr)
	}
	if err != nil {
		c.NotFoundOrError(err, "get label")
		return
	}

	c.JSONSuccess(toIssueLabel(label))
}

type createLabelRequest struct {
	Name  string `json:"name" binding:"Required"`
	Color string `json:"color" binding:"Required;Size(7)"`
}

func createLabel(c *apiContext, form createLabelRequest) {
	label := &database.Label{
		Name:   form.Name,
		Color:  form.Color,
		RepoID: c.Repo.Repository.ID,
	}
	if err := database.Handle.Labels().Create(c.Request().Context(), label); err != nil {
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
	label, err := database.Handle.Labels().GetByID(c.Request().Context(), c.Repo.Repository.ID, c.ParamInt64("id"))
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
	if err := database.Handle.Labels().Update(c.Request().Context(), label); err != nil {
		c.Error(err, "update label")
		return
	}
	c.JSONSuccess(toIssueLabel(label))
}

func deleteLabel(c *apiContext) {
	if err := database.Handle.Labels().DeleteByID(c.Request().Context(), c.Repo.Repository.ID, c.ParamInt64("id")); err != nil {
		c.Error(err, "delete label")
		return
	}

	c.NoContent()
}
